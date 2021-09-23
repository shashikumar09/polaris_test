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



type HPALimits struct {
    Min float64
    Max float64
}

type ResourceLimits struct {
    CPU float64
    Memory float64
}

type ResourceRequests struct {
    CPU float64
    Memory float64
}

type ResourceCost struct {
    CPU float64
    Memory float64
}

type ResourceUsage struct {
    CPU float64
    Memory float64
}
var (
    HPALimitsQuery string = "sum:kubernetes_state.hpa.max_replicas{*}by{hpa,kube_namespace,cluster_name}";
    ResourcesLimitsQuery  string = "avg:kubernetes.cpu.limits{*}by{kube_deployment,kube_namespace,cluster_name},avg:kubernetes.memory.limits{*}by{kube_deployment,kube_namespace,cluster_name}";
    ReplicasCountForDeploymentQuery string = "avg:kubernetes_state.deployment.replicas{*}by{kube_deployment,kube_namespace,cluster_name}";
    ReplicasCountQuery string = "sum:kubernetes_state.deployment.replicas{*}by{kube_deployment,cluster_name}";
    ResourceRequestsQuery string = "avg:kubernetes.cpu.requests{*}by{kube_deployment,kube_namespace,cluster_name}.rollup(avg, 2419200),avg:kubernetes.memory.requests{*}by{kube_deployment,kube_namespace,cluster_name}.rollup(avg, 2419200)";
    ResourceUsageQuery string = "avg:kubernetes.cpu.usage.total{*}by{kube_deployment,kube_namespace,cluster_name}.rollup(avg, 2419200),avg:kubernetes.memory.usage{*}by{kube_deployment,kube_namespace,cluster_name}.rollup(avg, 2419200)";

    ResourceLimitsForDeployment QueryResponse;
    HPALimitsForDeployment QueryResponse;
    ReplicasCountForDeployment QueryResponse;
    ReplicasCount QueryResponse;
    ResourceRequestsForDeployment QueryResponse;
    ResourceUsageForDeployment QueryResponse;
)

func init() {
    ResourceLimitsForDeployment = queryTSMetricsFromDatadog(ResourcesLimitsQuery)
    HPALimitsForDeployment = queryTSMetricsFromDatadog(HPALimitsQuery)
    ReplicasCountForDeployment = queryTSMetricsFromDatadog(ReplicasCountForDeploymentQuery)
    ReplicasCount = queryTSMetricsFromDatadog(ReplicasCountQuery)
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


func GetResourceLimitsForDeployment(deployment string, namespace string, cluster string) ResourceLimits {

    var resourceLimits ResourceLimits;
    var CPUMetric string = "kubernetes.cpu.limits"
    var MemoryMetric string = "kubernetes.memory.limits"
    for _,i:= range ResourceLimitsForDeployment.Series {
	if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:" + cluster {
	    fmt.Println(i.Metric)
            if i.Metric == CPUMetric {
                resourceLimits.CPU = i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format

            }else if i.Metric == MemoryMetric {

                resourceLimits.Memory = i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format

           }
        }
    }
    return resourceLimits
    
}
func GetResourceLimits(deployment string,  cluster string) ResourceLimits {

    var resourceLimits ResourceLimits;
    var CPUMetric string = "kubernetes.cpu.limits"
    var MemoryMetric string = "kubernetes.memory.limits"
    for _,i:= range ResourceLimitsForDeployment.Series {
	if  i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[1] == "cluster_name:" + cluster {
            if i.Metric == CPUMetric {
                resourceLimits.CPU = i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format

            }else if i.Metric == MemoryMetric {

                resourceLimits.Memory = i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format

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
            limit = i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format

	}
    }
    return limit
}

func FetchRepicasCountForDeployment(deployment string, namespace string, cluster string) float64 {
    
    var replicaCount float64;
    for _, i:= range ReplicasCountForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:" + cluster{
	    replicaCount =  i.PointList[len(i.PointList)-1][1] ///To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format
	    break;
        }
    }
    return replicaCount


}

func GetHPALimitsForDeployment(deployment string, namespace string, cluster string) HPALimits{
    //It will check whether HPA defined or not. If defined it will return min/max limits. else it will return min as 1 max as replicas configured for deplyoment
    var HPALimits HPALimits;
    HPALimits.Min = 1
    if isDeploymentContainsHPA(deployment, namespace, cluster) {
	limit := FetchHPALimitForDeployment(deployment, namespace, cluster)
        HPALimits.Max = limit
        return HPALimits

     } else {
        limit := FetchRepicasCountForDeployment(deployment, namespace, cluster)
        HPALimits.Max = limit
	    return HPALimits
    }

}

//To get the average of entire upper cluster(Inlcuding all namespaces where given deployment running)
func GetHPALimits(deployment string, cluster string) HPALimits {
    var HPALimits HPALimits;
    HPALimits.Min = 1
    for _, i:= range ReplicasCount.Series {
	    if i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[1] == "cluster_name:" + cluster{
	    HPALimits.Max =  i.PointList[len(i.PointList)-1][1] //To get the latest timestamp value. Pointlist stored in [<timestamp> <value>] format
	    break;
        }
    }
    return HPALimits
}




func getResourceRequestsForDeployment(deployment string, namespace string, cluster string) ResourceRequests {

    var resourceRequests ResourceRequests;
    var CPUMetric string = "kubernetes.cpu.requests"
    var MemoryMetric string = "kubernetes.memory.requests"
    for _,i:= range ResourceRequestsForDeployment.Series {
	    if i.TagSet[1] == "kube_namespace:" + namespace  && i.TagSet[0] == "kube_deployment:" + deployment && i.TagSet[2] == "cluster_name:"+ cluster {
	    fmt.Println(i.Metric)
            if i.Metric == CPUMetric {
                resourceRequests.CPU = i.PointList[0][1] //Pointlist stored in [<timestamp> <value>] format

            }else if i.Metric == MemoryMetric {

                resourceRequests.Memory = i.PointList[0][1] //Pointlist stored in [<timestamp> <value>] format

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
                resourceUsage.CPU = i.PointList[0][1] //Pointlist stored in [<timestamp> <value>] format

            }else if i.Metric == MemoryMetric {

                resourceUsage.Memory = i.PointList[0][1] //Pointlist stored in [<timestamp> <value>] format

           }
        }
    }
    return resourceUsage
    
} 
    
 func getGuranteedRequestsCost(deployment string, namespace string, cluster string, ResourceCostPerUnit ResourceCost) ResourceCost {
    //returns the guranteed usage by multiplying the requests defined with the cost per unit
    var GuaranteedRequestsCost ResourceCost;
    var resourceRequests ResourceRequests = getResourceRequestsForDeployment(deployment, namespace, cluster)
    GuaranteedRequestsCost.CPU = resourceRequests.CPU * ResourceCostPerUnit.CPU
    GuaranteedRequestsCost.Memory = (resourceRequests.Memory / 1073741824) * ResourceCostPerUnit.Memory
    return GuaranteedRequestsCost
        
}

    
func getActualUsageCost(deployment string, namespace string, cluster string, ResourceCostPerUnit ResourceCost) ResourceCost {
    
    //returns the actual usage by multiplying actual usage with the cost per unit
    var resourceUsage ResourceUsage = getResourceUsageForDeployment(deployment, namespace, cluster)
    var ActualUsageCost ResourceCost;
    ActualUsageCost.CPU = resourceUsage.CPU * ResourceCostPerUnit.CPU
    ActualUsageCost.Memory = (resourceUsage.Memory / 1073741824) * ResourceCostPerUnit.Memory

    return ActualUsageCost

}


func GetWastageCostForDeployment(deployment string, namespace string, cluster string, ResourceCostPerUnit ResourceCost) (ResourceCost, ResourceCost, ResourceCost) {
    //returns wastage for deployment

    var wastageCost ResourceCost;
    var GuaranteedRequestsCost ResourceCost = getGuranteedRequestsCost(deployment, namespace, cluster, ResourceCostPerUnit)
    var ActualUsageCost ResourceCost = getActualUsageCost(deployment, namespace, cluster, ResourceCostPerUnit)
    
    wastageCost.CPU = GuaranteedRequestsCost.CPU - ActualUsageCost.CPU
    wastageCost.Memory = GuaranteedRequestsCost.Memory - ActualUsageCost.Memory
    return wastageCost, GuaranteedRequestsCost, ActualUsageCost

}


