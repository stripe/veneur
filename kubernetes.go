package veneur

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KubernetesDiscoverer struct {
	clientset *kubernetes.Clientset
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
	pods, err := kd.clientset.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: "app=veneur-global",
	})
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {

		var forwardPort string

		// TODO don't assume there is only one container for the veneur global
		log.Debug("Found pod %#v", pod)
		log.Debug("Containers are %#v", pod.Spec.Containers)
		if len(pod.Spec.Containers) > 0 {
			for _, port := range pod.Spec.Containers[0].Ports {
				if port.Name == "http" {
					forwardPort = strconv.Itoa(int(port.HostPort))
					log.WithField("port", forwardPort).Debug("Found http port")
					break
				}

				// TODO don't assume all TCP ports are for importing
				if port.Protocol == "TCP" {
					forwardPort = strconv.Itoa(int(port.HostPort))
					log.WithField("port", forwardPort).Debug("Found TCP port")
				}
			}
		}

		if forwardPort == "" {
			log.Error("Could not find valid port for forwarding")
		}

		podIp := fmt.Sprintf("%s:%s", pod.Status.PodIP, forwardPort)
		ips = append(ips, podIp)
	}
	return ips, nil
}
