package datadog

import (
    "context"
    "fmt"
    "os"
    "log"
    "io/ioutil"
    "encoding/json"
    datadog "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
)

type queryResponse struct {
	Status string  `json:"status"`
    Series  []struct {
        Metric string `json:"metric"`
        TagSet []string `json:"tag_set"`
        PointList [][]float64 `json:"pointlist"`
        } `json:"series"`
}


type resourceLimits struct {
    cpu float64
    memory float64
}

var HPALimitsQuery string = "sum:kubernetes_state.hpa.max_replicas{cluster_name:prod--us-east-1--uat}by{hpa,kube_namespace}";

var ResourcesLimitsQuery  string = "avg:kubernetes.cpu.limits{cluster_name:prod--us-east-1--uat}by{kube_deployment,kube_namespace},avg:kubernetes.memory.limits{cluster_name:prod--us-east-1--uat}by{kube_deployment,kube_namespace}";
var ReplicasCountQuery string = "avg:kubernetes_state.deployment.replicas{cluster_name:prod--us-east-1--uat,kube_deployment:sshbg}by{kube_deployment,kube_namespace}";


var ResourceLimitsForDeployment queryResponse = queryTSMetricsFromDatadog(ResourcesLimitsQuery)
var HPALimitsForDeployment queryResponse = queryTSMetricsFromDatadog(HPALimitsQuery)
var ReplicasCountForDeployment queryResponse = queryTSMetricsFromDatadog(ReplicasCountQuery)


func queryTSMetricsFromDatadog(query string) queryResponse {
    ctx := context.WithValue(
        context.Background(),
        datadog.ContextAPIKeys,
        map[string]datadog.APIKey{
            "apiKeyAuth": {
                Key: os.Getenv("DD_CLIENT_API_KEY"),
            },
            "appKeyAuth": {
                Key: os.Getenv("DD_CLIENT_APP_KEY"),
            },
        },
    )	
    from := int64(1630483331) // int64 | Start of the queried time period, seconds since the Unix epoch.
    to := int64(1630483428)
    
    configuration := datadog.NewConfiguration()
    
    apiClient := datadog.NewAPIClient(configuration)
    resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, from, to, query)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `MetricsApi.ListTagsByMetricName`: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
        // response from `QueryMetrics`: 
    responseContent, _ := json.MarshalIndent(resp, "", "  ")
    fmt.Fprintf(os.Stdout, "Response from MetricsApi.ListTagsByMetricName:\n%s\n", responseContent)
    var queryResponse1 queryResponse
    defer r.Body.Close()
    bodyBytes, err := ioutil.ReadAll(r.Body)
    if err != nil {
        log.Fatal(err)
    }
    err1 := json.Unmarshal([]byte(bodyBytes), &queryResponse1)
    if err1 != nil {
	    log.Fatal(err1)
	   }

    //fmt.Println(queryResponse1)
    return queryResponse1

}


func getResourceLimitsForDeployment(deployment string, namespace string, cluster string) resourceLimits {

    var resourceLimits resourceLimits;
    var CPUMetric string = "kubernetes.cpu.limits"
    var MemoryMetric string = "kubernetes.memory.limits"
    for _,i:= range ResourceLimitsForDeployment.Series {
	if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment {
	    fmt.Println(i.Metric)
            if i.Metric == CPUMetric {
                resourceLimits.cpu = i.PointList[0][1]

            }else if i.Metric == MemoryMetric {

                resourceLimits.memory = i.PointList[0][1]

           }
        }
    }
    return resourceLimits
    
} 


func isDeploymentContainsHPA(deployment string, namespace string, cluster string) bool{

    for _, i:= range HPALimitsForDeployment.Series {
        if i.TagSet[1] == "kube_namespace:" + namespace && i.TagSet[0] == "hpa:hpa-" + deployment {
            return true
        }
        
	}
    return false
    
} 

func FetchHPALimitForDeployment(deployment string, namespace string, cluster string) float64 {
   
    var limit float64;
    for _, i:= range HPALimitsForDeployment.Series {
        if i.TagSet[1] == "kube_namespace:" + namespace && i.TagSet[0] == "hpa:hpa-" + deployment {
            limit = i.PointList[0][1]

	}
    }
    return limit
}

func FetchRepicasCountForDeployment(deployment string, namespace string, cluster string) float64 {
    
    var replicaCount float64;
    for _, i:= range ReplicasCountForDeployment.Series {
	if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment {
	    replicaCount =  i.PointList[0][1]
	    break;
        }
    }
    return replicaCount


}

func getHPALimitsForDeployment(deployment string, namespace string, cluster string) float64{
    //It will check whether HPA defined or not. If defined it will return min/max limits. else it will return min as 1 max as replicas configured for deplyoment
    
    if isDeploymentContainsHPA(deployment, namespace, cluster) {
	limit := FetchHPALimitForDeployment(deployment, namespace, cluster)
        return limit

     } else {

        limit := FetchRepicasCountForDeployment(deployment, namespace, cluster)
	return limit
    }
    }


func main(){

    limit := getHPALimitsForDeployment("sshbg", "shared-uat", "prod--us-east-1--uat")
    fmt.Println(limit)
    p := getResourceLimitsForDeployment("sshbg", "shared-uat", "prod--us-east-1--uat")
    fmt.Println(p.cpu, p.memory)

}