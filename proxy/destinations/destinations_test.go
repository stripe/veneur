package destinations_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/proxy/connect"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/scopedstatsd"
)

type TestDestinations struct {
	connect      *connect.MockConnect
	destinations destinations.Destinations
	logger       *logrus.Logger
	statsd       *scopedstatsd.MockClient
}

func CreateTestDestinations(
	ctrl *gomock.Controller, dialTimeout time.Duration,
) *TestDestinations {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockConnect := connect.NewMockConnect(ctrl)

	return &TestDestinations{
		connect: mockConnect,
		destinations: destinations.Create(
			mockConnect, logrus.NewEntry(logger)),
		logger: logger,
		statsd: mockStatsd,
	}
}

func TestAddEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	fixture.destinations.Add(context.Background(), []string{})

	assert.Zero(t, fixture.destinations.Size())
}

func TestAddSingle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address", fixture.destinations).
		Return(destination, nil)

	fixture.destinations.Add(context.Background(), []string{"address"})

	assert.Equal(t, 1, fixture.destinations.Size())

	actualDestination, err := fixture.destinations.Get("key")
	assert.NoError(t, err)
	assert.Equal(t, destination, actualDestination)
}

func TestAddMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination1 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address1", fixture.destinations).
		Return(destination1, nil)
	destination2 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address2", fixture.destinations).
		Return(destination2, nil)
	destination3 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address3", fixture.destinations).
		Return(destination3, nil)

	fixture.destinations.Add(context.Background(), []string{
		"address1", "address2", "address3",
	})

	assert.Equal(t, 3, fixture.destinations.Size())
}

func TestGetEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination, err := fixture.destinations.Get("sample-key")

	assert.Nil(t, destination)
	if assert.Error(t, err) {
		assert.Equal(t, "empty circle", err.Error())
	}
}

func TestClear(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination1 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address1", fixture.destinations).
		Return(destination1, nil)
	destination1.EXPECT().Close()
	destination2 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address2", fixture.destinations).
		Return(destination2, nil)
	destination2.EXPECT().Close()
	destination3 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address3", fixture.destinations).
		Return(destination3, nil)
	destination3.EXPECT().Close()

	fixture.destinations.Add(context.Background(), []string{
		"address1", "address2", "address3",
	})

	assert.Equal(t, 3, fixture.destinations.Size())

	fixture.destinations.Clear()
}

func TestRemoveDestination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination1 := connect.NewMockDestination(ctrl)
	var destinationHash connect.DestinationHash
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address1", fixture.destinations).
		DoAndReturn(func(
			_ context.Context, _ string, hash connect.DestinationHash,
		) (connect.Destination, error) {
			destinationHash = hash
			return destination1, nil
		})
	destination2 := connect.NewMockDestination(ctrl)
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address2", fixture.destinations).
		Return(destination2, nil)

	fixture.destinations.Add(context.Background(), []string{
		"address1", "address2",
	})
	destinationHash.RemoveDestination("address1")
	actualDestination, err := fixture.destinations.Get("key")

	assert.NoError(t, err)
	assert.Equal(t, 1, fixture.destinations.Size())
	assert.Equal(t, destination2, actualDestination)
}

func TestWait(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestDestinations(ctrl, 30*time.Second)
	destination := connect.NewMockDestination(ctrl)
	var destinationHash connect.DestinationHash
	fixture.connect.EXPECT().Connect(
		gomock.Any(), "address", fixture.destinations).
		DoAndReturn(func(
			_ context.Context, _ string, hash connect.DestinationHash,
		) (connect.Destination, error) {
			destinationHash = hash
			return destination, nil
		})

	fixture.destinations.Add(context.Background(), []string{"address"})
	destinationHash.ConnectionClosed()
	fixture.destinations.Wait()
}
