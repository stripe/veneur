// Code generated by MockGen. DO NOT EDIT.
// Source: discovery/discoverer.go

// Package discovery is a generated GoMock package.
package discovery

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockDiscoverer is a mock of Discoverer interface.
type MockDiscoverer struct {
	ctrl     *gomock.Controller
	recorder *MockDiscovererMockRecorder
}

// MockDiscovererMockRecorder is the mock recorder for MockDiscoverer.
type MockDiscovererMockRecorder struct {
	mock *MockDiscoverer
}

// NewMockDiscoverer creates a new mock instance.
func NewMockDiscoverer(ctrl *gomock.Controller) *MockDiscoverer {
	mock := &MockDiscoverer{ctrl: ctrl}
	mock.recorder = &MockDiscovererMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDiscoverer) EXPECT() *MockDiscovererMockRecorder {
	return m.recorder
}

// GetDestinationsForService mocks base method.
func (m *MockDiscoverer) GetDestinationsForService(arg0 string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDestinationsForService", arg0)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDestinationsForService indicates an expected call of GetDestinationsForService.
func (mr *MockDiscovererMockRecorder) GetDestinationsForService(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDestinationsForService", reflect.TypeOf((*MockDiscoverer)(nil).GetDestinationsForService), arg0)
}
