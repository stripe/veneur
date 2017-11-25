package veneur

import (
    "k8s.io/client-go/kubernetes"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/rest"
)

type KubernetesDiscoverer struct {
    clientset  *kubernetes.Clientset
}


func NewKubernetesDiscoverer() (*KubernetesDiscoverer, error) {
    // creates the in-cluster config
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, err
    }
    // creates the clientset
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, err
    }
    return &KubernetesDiscoverer{clientset}, nil
}


func (kd *KubernetesDiscoverer) GetDestinationsForService(serviceName string) ([]string, error) {
    pods, err := kd.clientset.CoreV1().Pods(serviceName).List(metav1.ListOptions{})
    if err != nil{
        return nil, err
    }

    ips := make([]string, 0, len(pods.Items))
    for _, pod := range pods.Items{
        ips = append(ips, pod.Status.PodIP)
    }
    return ips, nil
}
