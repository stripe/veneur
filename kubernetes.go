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

		// TODO don't assume there is only one container for the veneur global
		log.WithField("index", podIndex).Debugf("Found pod %#v", pod)
		log.WithField("index", podIndex).Debugf("Containers are %#v", pod.Spec.Containers)
		if len(pod.Spec.Containers) > 0 {
			var i int
			var container v1.Container
			for i, container = range pod.Spec.Containers {
				log.WithField("index", container).Debugf("Container %d is %#v", i, container)
				log.WithField("index", container).Debugf("Container ports are %#v", container.Ports)
				for _, port := range container.Ports {
					if port.Name == "http" {
						forwardPort = strconv.Itoa(int(port.HostPort))
						log.WithFields(logrus.Fields{
							"forwardPort": forwardPort,
							"hostPort":    port.HostPort,
						}).Debug("Found http port")
						break
					}

					// TODO don't assume all TCP ports are for importing
					if port.Protocol == "TCP" {
						forwardPort = strconv.Itoa(int(port.HostPort))
						log.WithField("port", forwardPort).Debug("Found TCP port")
					}
				}
			}
		}

		if forwardPort == "" {
			log.WithFields(logrus.Fields{
				"podIndex":    podIndex,
				"PodIP":       pod.Status.PodIP,
				"forwardPort": forwardPort,
			}).Error("Could not find valid port for forwarding")
			continue
		}

		podIp := fmt.Sprintf("%s:%s", pod.Status.PodIP, forwardPort)
		ips = append(ips, podIp)
	}
	return ips, nil
}
