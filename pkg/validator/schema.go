package validator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"github.com/qri-io/jsonschema"
	"github.com/thoas/go-funk"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/fairwindsops/polaris/pkg/config"
	"github.com/fairwindsops/polaris/pkg/kube"
	"github.com/fairwindsops/polaris/pkg/datadog"

)

type schemaTestCase struct {
	Target           config.TargetKind
	Resource         kube.GenericResource
	IsInitContianer  bool
	Container        *corev1.Container
	ResourceProvider *kube.ResourceProvider
}

func resolveCheck(conf *config.Configuration, checkID string, test schemaTestCase) (*config.SchemaCheck, error) {
	if !conf.DisallowExemptions && hasExemptionAnnotation(test.Resource.ObjectMeta, checkID) {
		return nil, nil
	}
	check, ok := conf.CustomChecks[checkID]
	if !ok {
		check, ok = config.BuiltInChecks[checkID]
		if !ok {
			check, ok = conf.DynamicCustomChecks[checkID]
			if !ok {
				return nil, fmt.Errorf("Check %s not found", checkID)
			}
		}
	}
	containerName := ""
	if test.Container != nil {
		containerName = test.Container.Name
	}
	if !conf.IsActionable(checkID, test.Resource.ObjectMeta, containerName) {
		return nil, nil
	}
	if !check.IsActionable(test.Target, test.Resource.Kind, test.IsInitContianer) {
		return nil, nil
	}
	if _, ok := conf.DynamicCustomChecks[checkID]; !ok {

		checkPtr, err := check.TemplateForResource(test.Resource.Resource.Object)
		if err != nil {
			return nil, err
		}
		return checkPtr, nil
	} else {
		return  &check, nil
	}
}

func makeResult(conf *config.Configuration, check *config.SchemaCheck, passes bool, issues []jsonschema.ValError) ResultMessage {
	details := []string{}
	for _, issue := range issues {
		details = append(details, issue.Message)
	}
	result := ResultMessage{
		ID:       check.ID,
		Severity: "warning",
		Category: check.Category,
		Success:  passes,
		// FIXME: need to fix the tests before adding this back
		//Details: details,
	}
	if passes {
		result.Message = check.SuccessMessage
	} else {
		result.Message = check.FailureMessage
	}
	fmt.Println("make Result", result.Message, result.ID, result.Severity, result.Category)
	fmt.Println(result)
	return result
}

const exemptionAnnotationKey = "polaris.fairwinds.com/exempt"
const exemptionAnnotationPattern = "polaris.fairwinds.com/%s-exempt"

func hasExemptionAnnotation(objMeta metaV1.Object, checkID string) bool {
	annot := objMeta.GetAnnotations()
	val := annot[exemptionAnnotationKey]
	if strings.ToLower(val) == "true" {
		return true
	}
	checkKey := fmt.Sprintf(exemptionAnnotationPattern, checkID)
	val = annot[checkKey]
	if strings.ToLower(val) == "true" {
		return true
	}
	return false
}

// ApplyAllSchemaChecksToResourceProvider applies all available checks to a ResourceProvider
func ApplyAllSchemaChecksToResourceProvider(conf *config.Configuration, resourceProvider *kube.ResourceProvider) ([]Result, error) {
	results := []Result{}
	for _, resources := range resourceProvider.Resources {
		kindResults, err := ApplyAllSchemaChecksToAllResources(conf, resourceProvider, resources)
		if err != nil {
			return results, err
		}
		results = append(results, kindResults...)
	}
	return results, nil
}

// ApplyAllSchemaChecksToAllResources applies available checks to a list of resources
func ApplyAllSchemaChecksToAllResources(conf *config.Configuration, resourceProvider *kube.ResourceProvider, resources []kube.GenericResource) ([]Result, error) {
	results := []Result{}
	for _, resource := range resources {
		result, err := ApplyAllSchemaChecks(conf, resourceProvider, resource)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// ApplyAllSchemaChecks applies available checks to a single resource
func ApplyAllSchemaChecks(conf *config.Configuration, resourceProvider *kube.ResourceProvider, resource kube.GenericResource) (Result, error) {
	if resource.PodSpec == nil {
		return applyNonControllerSchemaChecks(conf, resourceProvider, resource)
	}
	return applyControllerSchemaChecks(conf, resourceProvider, resource)
}

func applyNonControllerSchemaChecks(conf *config.Configuration, resourceProvider *kube.ResourceProvider, resource kube.GenericResource) (Result, error) {
	finalResult := Result{
		Kind:      resource.Kind,
		Name:      resource.ObjectMeta.GetName(),
		Namespace: resource.ObjectMeta.GetNamespace(),
	}
	resultSet, err := applyTopLevelSchemaChecks(conf, resourceProvider, resource, false)
	finalResult.Results = resultSet
	return finalResult, err
}

func applyControllerSchemaChecks(conf *config.Configuration, resourceProvider *kube.ResourceProvider, resource kube.GenericResource) (Result, error) {
	finalResult := Result{
		Kind:      resource.Kind,
		Name:      resource.ObjectMeta.GetName(),
		Namespace: resource.ObjectMeta.GetNamespace(),
	}
	resultSet, err := applyTopLevelSchemaChecks(conf, resourceProvider, resource, true)
	if err != nil {
		return finalResult, err
	}
	finalResult.Results = resultSet

	podRS, err := applyPodSchemaChecks(conf, resourceProvider, resource)
	if err != nil {
		return finalResult, err
	}
	podRes := PodResult{
		Results:          podRS,
		ContainerResults: []ContainerResult{},
	}
	finalResult.PodResult = &podRes

	for _, container := range resource.PodSpec.InitContainers {
		results, err := applyContainerSchemaChecks(conf, resourceProvider, resource, &container, true)
		if err != nil {
			return finalResult, err
		}
		cRes := ContainerResult{
			Name:    container.Name,
			Results: results,
		}
		podRes.ContainerResults = append(podRes.ContainerResults, cRes)
	}
	for _, container := range resource.PodSpec.Containers {
		results, err := applyContainerSchemaChecks(conf, resourceProvider, resource, &container, false)
		if err != nil {
			return finalResult, err
		}
		cRes := ContainerResult{
			Name:    container.Name,
			Results: results,
		}
		podRes.ContainerResults = append(podRes.ContainerResults, cRes)
	}

	return finalResult, nil
}

func applyTopLevelSchemaChecks(conf *config.Configuration, resources *kube.ResourceProvider, res kube.GenericResource, isController bool) (ResultSet, error) {
	test := schemaTestCase{
		ResourceProvider: resources,
		Resource:         res,
	}
	if isController {
		test.Target = config.TargetController
	}
	return applySchemaChecks(conf, test)
}

func applyPodSchemaChecks(conf *config.Configuration, resources *kube.ResourceProvider, controller kube.GenericResource) (ResultSet, error) {
	test := schemaTestCase{
		Target:           config.TargetPod,
		ResourceProvider: resources,
		Resource:         controller,
	}
	return applySchemaChecks(conf, test)
}

func applyContainerSchemaChecks(conf *config.Configuration, resources *kube.ResourceProvider, controller kube.GenericResource, container *corev1.Container, isInit bool) (ResultSet, error) {
	test := schemaTestCase{
		Target:           config.TargetContainer,
		ResourceProvider: resources,
		Resource:         controller,
		Container:        container,
		IsInitContianer:  isInit,
	}
	return applySchemaChecks(conf, test)
}

func applySchemaChecks(conf *config.Configuration, test schemaTestCase) (ResultSet, error) {
	results := ResultSet{}
	checkIDs := getSortedKeys(conf.Checks)
	for _, checkID := range checkIDs {
		if _, ok := conf.DynamicCustomChecks[checkID]; ok {
			result, err := applyDynamicSchemaCheck(conf, checkID, test)
			if err != nil {
				return results, err
			}
		
			if result != nil {
				results[checkID] = *result
			}
		} else {
			result, err := applySchemaCheck(conf, checkID, test);
			if err != nil {
				return results, err
			}

			if result != nil {
				results[checkID] = *result
			}
		}
		}
	
	return results, nil
}



func makeDynamicResult(conf *config.Configuration, check *config.SchemaCheck, passes bool, issues []jsonschema.ValError) ResultMessage {
	details := []string{}
	for _, issue := range issues {
		details = append(details, issue.Message)
	}
	result := ResultMessage{
		ID:       check.ID,
		Severity: conf.Checks[check.ID],
		Category: check.Category,
		Success:  passes,
		// FIXME: need to fix the tests before adding this back
		//Details: details,
	}
	if passes {
		result.Message = check.SuccessMessage
	} else {
		result.Message = check.FailureMessage
	}
	return result
}


func HandleHPALimitsCheck(checkID string, test schemaTestCase) (bool, []jsonschema.ValError, error){
	var expectedLimits  float64;     
	//var actualLimits float64;
	if os.Getenv("MACHINE_STABILITY") == "dev" || os.Getenv("MACHINE_STABILITY") == "qa" {
		expectedLimits = 3 // datadog.getHPALimitsForDeqaDeployment(test.Resource.ObjectMeta.GetName(),test.Resource.ObjectMeta.GetNamespace(), os.Getenv("UPPER_CLUSTER"))
	} else if os.Getenv("MACHINE_STABILITY") == "uat" {
		expectedLimits = datadog.GetHPALimitsForDeployment(test.Resource.ObjectMeta.GetName(), strings.ReplaceAll(test.Resource.ObjectMeta.GetNamespace(), "uat", ""), os.Getenv("CLUSTER"))
	} else {
		expectedLimits = 2
	}
	actualLimits  := datadog.GetHPALimitsForDeployment(test.Resource.ObjectMeta.GetName(), test.Resource.ObjectMeta.GetNamespace(), os.Getenv("MACHINE_STABILITY"))
	if int(actualLimits) < int(expectedLimits) {
		return true, nil, nil
	} else {
		return false, nil, nil
	}
		
}


func HandleWastageCostCheck(check *config.SchemaCheck, checkID string, test schemaTestCase) (bool,   []jsonschema.ValError, error) {
	wastageCost, guaranteedUsageCost, actualUsageCost := datadog.GetWastageCostForDeployment(test.Resource.ObjectMeta.GetName(), test.Resource.ObjectMeta.GetNamespace(), os.Getenv("UPPER_CLUSTER"))
	wastageCostLimit, ok := check.Schema["limit"].(float64); 
	if !ok {
 	   return false, nil,fmt.Errorf("Limits not found for schema", wastageCostLimit) 
	}
	totalGuranteedUsageCost := guaranteedUsageCost.Memory + guaranteedUsageCost.CPU
	totalWastageCost := wastageCost.CPU + wastageCost.Memory
	if int(totalWastageCost) > int(wastageCostLimit) {
		message := "wastage cost is $" + fmt.Sprintf("%f", totalWastageCost) + "/m (Memory/CPU requests - Actual usage)($" + fmt.Sprintf("%f" , totalGuranteedUsageCost) + " -$" + fmt.Sprintf("%f", actualUsageCost)+ ")"
		check.FailureMessage = message
		return false,  nil, nil
	} else {
		message := "No wastage"
		check.SuccessMessage = message
		return true, nil, nil
	}
}

func HandleResourceLimitsCheck(check *config.SchemaCheck, checkID string, test schemaTestCase) (bool, []jsonschema.ValError, error) {
	//var actualLimits float64;
	var expectedResourceLimits datadog.ResourceLimits
	if os.Getenv("MACHINE_STABILITY") == "dev" || os.Getenv("MACHINE_STABILITY") == "qa" {
		expectedResourceLimits = datadog.GetResourceLimitsForDeployment(test.Resource.ObjectMeta.GetName(),test.Resource.ObjectMeta.GetNamespace(), os.Getenv("UPPER_CLUSTER"))
	} else  {
		expectedResourceLimits = datadog.GetResourceLimitsForDeployment(test.Resource.ObjectMeta.GetName(), strings.ReplaceAll(test.Resource.ObjectMeta.GetNamespace(), "uat", ""), os.Getenv("CLUSTER"))
	} 
	fmt.Println(expectedResourceLimits)
	actualResourceLimits  := datadog.GetResourceLimitsForDeployment(test.Resource.ObjectMeta.GetName(), test.Resource.ObjectMeta.GetNamespace(), os.Getenv("CLUSTER"))
	if int(actualResourceLimits.CPU) > int(expectedResourceLimits.CPU) && int(actualResourceLimits.Memory) > int(expectedResourceLimits.Memory) {
		message := "CPU/Memory Limits are not within range"
		check.FailureMessage = message
		return false, nil, nil
	} else if int(actualResourceLimits.CPU) > int(expectedResourceLimits.CPU) {
		message := "CPU Limits are not within range"
		check.FailureMessage = message
		return false, nil, nil
	} else if int(actualResourceLimits.Memory) > int(expectedResourceLimits.Memory) {
		message := "Memory Limits are not within range"
		check.FailureMessage = message
		return false, nil, nil
	} else if int(actualResourceLimits.CPU) <= int(expectedResourceLimits.CPU) || int(actualResourceLimits.Memory) <= int(expectedResourceLimits.Memory) {
		message := "CPU/Memory Limits are within range"
		check.FailureMessage = message
		return true, nil, nil
	} else if int(actualResourceLimits.CPU) <= int(expectedResourceLimits.CPU) {
		message := "CPU Limits within range"
		check.FailureMessage = message
		return true, nil, nil
	} else {

		message := "Memory Limits within range"
		check.FailureMessage = message
		return true, nil, nil
	}


}

func applyDynamicSchemaCheck(conf *config.Configuration, checkID string, test schemaTestCase) (*ResultMessage, error) {
	// Will perform DynamicSchemaChecks

	//check, err := resolveCheck(conf, checkID, test)
	check, err := resolveCheck(conf, checkID, test)
	if err != nil {
		return nil, err
	} else if check == nil {
		return nil, nil
	}
	var passes bool
	fmt.Println("I'm in dynamic check function", checkID)
	var issues []jsonschema.ValError
	if checkID == "HPALimits" {
		passes, issues, err = HandleHPALimitsCheck(checkID, test)
		if err != nil {
			return nil, err
		}
	} else if checkID == "WastageCost" {
		passes, issues, err = HandleWastageCostCheck(check, checkID, test)
		if err != nil {
			return nil, err
		}
	} else if checkID == "ResourceLimits" {
		passes, issues, err = HandleResourceLimitsCheck(check, checkID, test)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, nil
	}
	result := makeResult(conf, check, passes, issues)
	return &result, nil		
}

func applySchemaCheck(conf *config.Configuration, checkID string, test schemaTestCase) (*ResultMessage, error) {
	check, err := resolveCheck(conf, checkID, test)
	if err != nil {
		return nil, err
	} else if check == nil {
		return nil, nil
	}
	var passes bool
	var issues []jsonschema.ValError
	if check.SchemaTarget != "" {
		if check.SchemaTarget == config.TargetPod && check.Target == config.TargetContainer {
			podCopy := *test.Resource.PodSpec
			podCopy.InitContainers = []corev1.Container{}
			podCopy.Containers = []corev1.Container{*test.Container}
			passes, issues, err = check.CheckPod(&podCopy)
		} else {
			return nil, fmt.Errorf("Unknown combination of target (%s) and schema target (%s)", check.Target, check.SchemaTarget)
		}
	} else if check.Target == config.TargetPod {
		passes, issues, err = check.CheckPod(test.Resource.PodSpec)
	} else if check.Target == config.TargetContainer {
		passes, issues, err = check.CheckContainer(test.Container)
	} else {
		passes, issues, err = check.CheckObject(test.Resource.Resource.Object)
	}
	if err != nil {
		return nil, err
	}
	for groupkind := range check.AdditionalValidators {
		if !passes {
			break
		}
		resources := test.ResourceProvider.Resources[groupkind]
		namespace := test.Resource.ObjectMeta.GetNamespace()
		if test.Resource.Kind == "Namespace" {
			namespace = test.Resource.ObjectMeta.GetName()
		}
		resources = funk.Filter(resources, func(res kube.GenericResource) bool {
			return res.ObjectMeta.GetNamespace() == namespace
		}).([]kube.GenericResource)
		objects := funk.Map(resources, func(res kube.GenericResource) interface{} {
			return res.Resource.Object
		}).([]interface{})
		passes, err = check.CheckAdditionalObjects(groupkind, objects)
		if err != nil {
			return nil, err
		}
	}
	result := makeResult(conf, check, passes, issues)
	return &result, nil
}

func getSortedKeys(m map[string]config.Severity) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
