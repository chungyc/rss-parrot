// Code generated by MockGen. DO NOT EDIT.
// Source: rss_parrot/logic (interfaces: IBlockedFeeds)
//
// Generated by this command:
//
//	mockgen --build_flags=--mod=mod -destination ../test/mocks/mock_blocked_feeds.go -package mocks rss_parrot/logic IBlockedFeeds
//

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockIBlockedFeeds is a mock of IBlockedFeeds interface.
type MockIBlockedFeeds struct {
	ctrl     *gomock.Controller
	recorder *MockIBlockedFeedsMockRecorder
}

// MockIBlockedFeedsMockRecorder is the mock recorder for MockIBlockedFeeds.
type MockIBlockedFeedsMockRecorder struct {
	mock *MockIBlockedFeeds
}

// NewMockIBlockedFeeds creates a new mock instance.
func NewMockIBlockedFeeds(ctrl *gomock.Controller) *MockIBlockedFeeds {
	mock := &MockIBlockedFeeds{ctrl: ctrl}
	mock.recorder = &MockIBlockedFeedsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIBlockedFeeds) EXPECT() *MockIBlockedFeedsMockRecorder {
	return m.recorder
}

// IsBlocked mocks base method.
func (m *MockIBlockedFeeds) IsBlocked(arg0 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsBlocked", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsBlocked indicates an expected call of IsBlocked.
func (mr *MockIBlockedFeedsMockRecorder) IsBlocked(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsBlocked", reflect.TypeOf((*MockIBlockedFeeds)(nil).IsBlocked), arg0)
}