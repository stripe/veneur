package veneur

import (
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
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
	for podIndex, pod := range pods.Items {

		var forwardPort string

		if pod.Status.Phase != v1.PodRunning {
			continue
		}

		// TODO don't assume there is only one container for the veneur global
		if len(pod.Spec.Containers) > 0 {
			for _, container := range pod.Spec.Containers {
				for _, port := range container.Ports {
					if port.Name == "http" {
						forwardPort = strconv.Itoa(int(port.ContainerPort))
						log.WithField("port", forwardPort).Debug("Found http port")
						break
					}

					// TODO don't assume all TCP ports are for importing
					if port.Protocol == "TCP" {
						forwardPort = strconv.Itoa(int(port.ContainerPort))
						log.WithField("port", forwardPort).Debug("Found TCP port")
					}
				}
			}
		}

		if forwardPort == "" || forwardPort == "0" {
			log.WithFields(logrus.Fields{
				"podIndex":    podIndex,
				"PodIP":       pod.Status.PodIP,
				"forwardPort": forwardPort,
			}).Error("Could not find valid port for forwarding")
			continue
		}

		if pod.Status.PodIP == "" {
			log.WithFields(logrus.Fields{
				"podIndex":    podIndex,
				"PodIP":       pod.Status.PodIP,
				"forwardPort": forwardPort,
			}).Error("Could not find valid podIP for forwarding")
			continue
		}

		// prepend with // so that it is a valid URL parseable by url.Parse
		podIp := fmt.Sprintf("http://%s:%s", pod.Status.PodIP, forwardPort)
		ips = append(ips, podIp)
	}
	return ips, nil
}
