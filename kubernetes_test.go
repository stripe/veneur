package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

func TestGetDestinationFromHttpPod(t *testing.T) {
	httpPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Ports: []v1.ContainerPort{
						v1.ContainerPort{
							Name:          "http",
							ContainerPort: 8080,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
	}

	podID := getDestinationFromPod(0, httpPod)
	assert.Equal(t, "http://127.0.0.1:8080", podID)
}

func TestGetDestinationFromTcpPod(t *testing.T) {
	tcpPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Ports: []v1.ContainerPort{
						v1.ContainerPort{
							Name:          "tcp",
							ContainerPort: 8080,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
	}

	podID := getDestinationFromPod(0, tcpPod)
	assert.Equal(t, "http://127.0.0.1:8080", podID)
}

func TestGetDestinationFromGrpcPod(t *testing.T) {
	grpcPod := v1.Pod{
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "127.0.0.1",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Ports: []v1.ContainerPort{
						v1.ContainerPort{
							Name:          "grpc",
							ContainerPort: 8080,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
	}

	podID := getDestinationFromPod(0, grpcPod)
	assert.Equal(t, "127.0.0.1:8080", podID)
}
