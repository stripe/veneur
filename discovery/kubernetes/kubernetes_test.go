package kubernetes_test

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/discovery/kubernetes"
	v1 "k8s.io/api/core/v1"
)

func TestGetDestinationFromHttpPod(t *testing.T) {
	discoverer := kubernetes.KubernetesDiscoverer{
		Logger: logrus.NewEntry(logrus.New()),
	}
	httpPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Ports: []v1.ContainerPort{{
					Name:          "http",
					ContainerPort: 8080,
					Protocol:      v1.ProtocolTCP,
				}},
			},
			},
		},
	}

	podID := discoverer.GetDestinationFromPod(0, httpPod)
	assert.Equal(t, "http://127.0.0.1:8080", podID)
}

func TestGetDestinationFromTcpPod(t *testing.T) {
	discoverer := kubernetes.KubernetesDiscoverer{
		Logger: logrus.NewEntry(logrus.New()),
	}
	tcpPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Ports: []v1.ContainerPort{{
					Name:          "tcp",
					ContainerPort: 8080,
					Protocol:      v1.ProtocolTCP,
				}},
			}},
		},
	}

	podID := discoverer.GetDestinationFromPod(0, tcpPod)
	assert.Equal(t, "http://127.0.0.1:8080", podID)
}

func TestGetDestinationFromGrpcPod(t *testing.T) {
	discoverer := kubernetes.KubernetesDiscoverer{
		Logger: logrus.NewEntry(logrus.New()),
	}
	grpcPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Ports: []v1.ContainerPort{{
					Name:          "grpc",
					ContainerPort: 8080,
					Protocol:      v1.ProtocolTCP,
				}},
			}},
		},
	}

	podID := discoverer.GetDestinationFromPod(0, grpcPod)
	assert.Equal(t, "127.0.0.1:8080", podID)
}
