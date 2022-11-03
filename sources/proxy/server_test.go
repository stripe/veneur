package proxy_test

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/sources/mock"
	"github.com/stripe/veneur/v14/sources/proxy"
	"google.golang.org/grpc"
)

func TestName(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	server := proxy.New("localhost:0", logger)

	assert.Equal(t, "proxy", server.Name())
}

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIngest := mock.NewMockIngest(ctrl)

	logger := logrus.NewEntry(logrus.New())
	server := proxy.New("localhost:0", logger)

	go server.Start(mockIngest)
	<-server.Ready()
	server.Stop()
}

func TestSendMetricsV2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIngest := mock.NewMockIngest(ctrl)
	metric := metricpb.Metric{
		Name: "test-metric",
		Tags: []string{"tag1:value1", "tag2:value2"},
		Type: metricpb.Type_Counter,
		Value: &metricpb.Metric_Counter{
			Counter: &metricpb.CounterValue{
				Value: 10,
			},
		},
	}

	logger := logrus.NewEntry(logrus.New())
	server := proxy.New("localhost:0", logger)

	go server.Start(mockIngest)
	<-server.Ready()

	connection, err := grpc.Dial(server.GetAddress(), grpc.WithInsecure())
	assert.NoError(t, err)
	client := forwardrpc.NewForwardClient(connection)

	mockIngest.EXPECT().IngestMetricProto(&metric).Times(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sendClient, err := client.SendMetricsV2(ctx)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		err = sendClient.Send(&metric)
		assert.NoError(t, err)
	}

	_, err = sendClient.CloseAndRecv()
	assert.NoError(t, err)

	server.Stop()
}

func TestSendMetricsV2ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIngest := mock.NewMockIngest(ctrl)

	logger := logrus.NewEntry(logrus.New())
	server := proxy.New("localhost:0", logger)

	go server.Start(mockIngest)
	<-server.Ready()

	connection, err := grpc.Dial(server.GetAddress(), grpc.WithInsecure())
	assert.NoError(t, err)
	client := forwardrpc.NewForwardClient(connection)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sendClient, err := client.SendMetricsV2(ctx)
	assert.Error(t, err)
	assert.Nil(t, sendClient)

	server.Stop()
}
