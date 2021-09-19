package datadog

import (
    "context"
    "fmt"
    "os"
    "log"
    "io/ioutil"
    "encoding/json"
    datadogClient "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
)

var (
    CPUCostPerCore float64 = 23
    MemoryCostPerGB float64 = 6.5
)

type QueryResponse struct {
	Status string  `json:"status"`
    Series  []struct {
        Metric string `json:"metric"`
        TagSet []string `json:"tag_set"`
        PointList [][]float64 `json:"pointlist"`
        } `json:"series"`
}


type ResourceLimits struct {
    cpu float64
    memory float64
}

type ResourceRequests struct {
    cpu float64
    memory float64
}

type ResourceCost struct {
    cpu float64
    memory float64
}

type ResourceUsage struct {
    cpu float64
    memory float64
}
var (
    HPALimitsQuery string = "sum:kubernetes_state.hpa.max_replicas{*}by{hpa,kube_namespace,cluster_name}";
    ResourcesLimitsQuery  string = "avg:kubernetes.cpu.limits{*}by{kube_deployment,kube_namespace,cluster_name},avg:kubernetes.memory.limits{*}by{kube_deployment,kube_namespace,cluster_name}";
    ReplicasCountQuery string = "avg:kubernetes_state.deployment.replicas{*}by{kube_deployment,kube_namespace,cluster_name}";
    ResourceRequestsQuery string = "avg:kubernetes.cpu.requests{*}by{kube_deployment,kube_namespace,cluster_name},avg:kubernetes.memory.requests{*}by{kube_deployment,kube_namespace,cluster_name}";
    ResourceUsageQuery string = "avg:kubernetes.cpu.usage.total{*}by{kube_deployment,kube_namespace,cluster_name},avg:kubernetes.memory.usage{*}by{kube_deployment,kube_namespace,cluster_name}";

    ResourceLimitsForDeployment QueryResponse;
    HPALimitsForDeployment QueryResponse;
    ReplicasCountForDeployment QueryResponse;
    ResourceRequestsForDeployment QueryResponse;
    ResourceUsageForDeployment QueryResponse;
)

func init() {
    ResourceLimitsForDeployment = queryTSMetricsFromDatadog(ResourcesLimitsQuery)
    HPALimitsForDeployment = queryTSMetricsFromDatadog(HPALimitsQuery)
    ReplicasCountForDeployment = queryTSMetricsFromDatadog(ReplicasCountQuery)
    ResourceRequestsForDeployment = queryTSMetricsFromDatadog(ResourceRequestsQuery)
    ResourceUsageForDeployment = queryTSMetricsFromDatadog(ResourceUsageQuery)
}

func queryTSMetricsFromDatadog(query string) QueryResponse {
    ctx := context.WithValue(
        context.Background(),
        datadogClient.ContextAPIKeys,
        map[string]datadogClient.APIKey{
            "apiKeyAuth": {
                Key: os.Getenv("DD_CLIENT_API_KEY"),
            },
            "appKeyAuth": {
                Key: os.Getenv("DD_CLIENT_APP_KEY"),
            },
        },
    )
    fmt.Println(query)
    from := int64(1630483331) // int64 | Start of the queried time period, seconds since the Unix epoch.
    to := int64(1630483428)
    
    configuration := datadogClient.NewConfiguration()
    
    apiClient := datadogClient.NewAPIClient(configuration)
    resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, from, to, query)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `MetricsApi.ListTagsByMetricName`: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `QueryMetrics`: 
    json.MarshalIndent(resp, "", "  ")
    //responseContent, _ := json.MarshalIndent(resp, "", "  ")
    //fmt.Fprintf(os.Stdout, "Response from MetricsApi.ListTagsByMetricName:\n%s\n", responseContent)
    var queryResponse1 QueryResponse
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


func getResourceLimitsForDeployment(deployment string, namespace string, cluster string) ResourceLimits {

    var resourceLimits ResourceLimits;
    var CPUMetric string = "kubernetes.cpu.limits"
    var MemoryMetric string = "kubernetes.memory.limits"
    for _,i:= range ResourceLimitsForDeployment.Series {
	if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:" + cluster {
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
	    if i.TagSet[1] == "kube_namespace:" + namespace && i.TagSet[0] == "hpa:hpa-" + deployment && i.TagSet[2] == "cluster_name:" + cluster {
            return true
        }
        
	}
    return false
    
} 

func FetchHPALimitForDeployment(deployment string, namespace string, cluster string) float64 {
   
    var limit float64;
    for _, i:= range HPALimitsForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace && i.TagSet[0] == "hpa:hpa-" + deployment && i.TagSet[2] == "cluster_name:" + cluster {
            limit = i.PointList[0][1]

	}
    }
    return limit
}

func FetchRepicasCountForDeployment(deployment string, namespace string, cluster string) float64 {
    
    var replicaCount float64;
    for _, i:= range ReplicasCountForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:" + cluster{
	    replicaCount =  i.PointList[0][1]
	    break;
        }
    }
    return replicaCount


}

func GetHPALimitsForDeployment(deployment string, namespace string, cluster string) float64{
    //It will check whether HPA defined or not. If defined it will return min/max limits. else it will return min as 1 max as replicas configured for deplyoment
    
    if isDeploymentContainsHPA(deployment, namespace, cluster) {
	limit := FetchHPALimitForDeployment(deployment, namespace, cluster)
        return limit

     } else {
	fmt.Println("came to else block", deployment, namespace)
        limit := FetchRepicasCountForDeployment(deployment, namespace, cluster)
	fmt.Println(limit)
	return limit
    }

}


func getResourceRequestsForDeployment(deployment string, namespace string, cluster string) ResourceRequests {

    var resourceRequests ResourceRequests;
    var CPUMetric string = "kubernetes.cpu.requests"
    var MemoryMetric string = "kubernetes.memory.requests"
    for _,i:= range ResourceRequestsForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:"+ cluster {
	    fmt.Println(i.Metric)
            if i.Metric == CPUMetric {
                resourceRequests.cpu = i.PointList[0][1]

            }else if i.Metric == MemoryMetric {

                resourceRequests.memory = i.PointList[0][1]

           }
        }
    }
    return resourceRequests
    
} 

func getResourceUsageForDeployment(deployment string, namespace string, cluster string) ResourceUsage{
    var resourceUsage ResourceUsage;
    var CPUMetric string = "kubernetes.cpu.usage.total"
    var MemoryMetric string = "kubernetes.memory.usage"
    for _,i:= range ResourceUsageForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:" + cluster {
	    fmt.Println(i.Metric)
            if i.Metric == CPUMetric {
                resourceUsage.cpu = i.PointList[0][1]

            }else if i.Metric == MemoryMetric {

                resourceUsage.memory = i.PointList[0][1]

           }
        }
    }
    return resourceUsage
    
} 
    
 func getGuranteedRequestsCost(deployment string, namespace string, cluster string) ResourceCost {
    //returns the guranteed usage by multiplying the requests defined with the cost per unit
    var GuaranteedRequestsCost ResourceCost;
    var resourceRequests ResourceRequests = getResourceRequestsForDeployment(deployment, namespace, cluster)
    GuaranteedRequestsCost.cpu = resourceRequests.cpu * CPUCostPerCore
    GuaranteedRequestsCost.memory = (resourceRequests.memory / 1073741824) * MemoryCostPerGB

    return GuaranteedRequestsCost
        
}

    
func getActualUsageCost(deployment string, namespace string, cluster string) ResourceCost {
    
    //returns the actual usage by multiplying actual usage with the cost per unit
    var resourceUsage ResourceUsage = getResourceUsageForDeployment(deployment, namespace, cluster)
    var ActualUsageCost ResourceCost;
    ActualUsageCost.cpu = resourceUsage.cpu * CPUCostPerCore
    ActualUsageCost.memory = (resourceUsage.memory / 1073741824) * MemoryCostPerGB

    return ActualUsageCost

}


func GetWastageCostForDeployment(deployment string, namespace string, cluster string) (ResourceCost, ResourceCost, ResourceCost) {
    //returns wastage for deployment

    var wastageCost ResourceCost;
    var GuaranteedRequestsCost ResourceCost = getGuranteedRequestsCost(deployment, namespace, cluster)
    var ActualUsageCost ResourceCost = getActualUsageCost(deployment, namespace, cluster)
    
    wastageCost.cpu = GuaranteedRequestsCost.cpu - ActualUsageCost.cpu
    wastageCost.memory = GuaranteedRequestsCost.memory - ActualUsageCost.memory
    return wastageCost, GuaranteedRequestsCost, ActualUsageCost

}


