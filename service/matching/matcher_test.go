// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package matching

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/suite"
	querypb "go.temporal.io/temporal-proto/query"
	tasklistpb "go.temporal.io/temporal-proto/tasklist"
	"go.uber.org/atomic"

	commongenpb "github.com/temporalio/temporal/.gen/proto/common"
	"github.com/temporalio/temporal/.gen/proto/matchingservice"
	"github.com/temporalio/temporal/.gen/proto/matchingservicemock"
	"github.com/temporalio/temporal/.gen/proto/persistenceblobs"
	"github.com/temporalio/temporal/common/cache"
	"github.com/temporalio/temporal/common/metrics"
	"github.com/temporalio/temporal/common/persistence"
	"github.com/temporalio/temporal/common/primitives/timestamp"
	"github.com/temporalio/temporal/common/service/dynamicconfig"
)

type MatcherTestSuite struct {
	suite.Suite
	controller  *gomock.Controller
	client      *matchingservicemock.MockMatchingServiceClient
	fwdr        *Forwarder
	cfg         *taskListConfig
	taskList    *taskListID
	matcher     *TaskMatcher // matcher for child partition
	rootMatcher *TaskMatcher // matcher for parent partition
}

func TestMatcherSuite(t *testing.T) {
	suite.Run(t, new(MatcherTestSuite))
}

func (t *MatcherTestSuite) SetupTest() {
	t.controller = gomock.NewController(t.T())
	t.client = matchingservicemock.NewMockMatchingServiceClient(t.controller)
	cfg := NewConfig(dynamicconfig.NewNopCollection())
	t.taskList = newTestTaskListID(uuid.New(), taskListPartitionPrefix+"tl0/1", persistence.TaskListTypeDecision)
	tlCfg, err := newTaskListConfig(t.taskList, cfg, t.newNamespaceCache())
	t.NoError(err)
	tlCfg.forwarderConfig = forwarderConfig{
		ForwarderMaxOutstandingPolls: func() int { return 1 },
		ForwarderMaxOutstandingTasks: func() int { return 1 },
		ForwarderMaxRatePerSecond:    func() int { return 2 },
		ForwarderMaxChildrenPerNode:  func() int { return 20 },
	}
	t.cfg = tlCfg
	scope := func() metrics.Scope { return metrics.NoopScope(metrics.Matching) }
	t.fwdr = newForwarder(&t.cfg.forwarderConfig, t.taskList, tasklistpb.TaskListKindNormal, t.client, scope)
	t.matcher = newTaskMatcher(tlCfg, t.fwdr, func() metrics.Scope { return metrics.NoopScope(metrics.Matching) })

	rootTaskList := newTestTaskListID(t.taskList.namespaceID, t.taskList.Parent(20), persistence.TaskListTypeDecision)
	rootTasklistCfg, err := newTaskListConfig(rootTaskList, cfg, t.newNamespaceCache())
	t.NoError(err)
	t.rootMatcher = newTaskMatcher(rootTasklistCfg, nil, func() metrics.Scope { return metrics.NoopScope(metrics.Matching) })
}

func (t *MatcherTestSuite) TearDownTest() {
	t.controller.Finish()
}

func (t *MatcherTestSuite) TestLocalSyncMatch() {
	// force disable remote forwarding
	<-t.fwdr.AddReqTokenC()
	<-t.fwdr.PollReqTokenC()

	pollStarted := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		close(pollStarted)
		task, err := t.matcher.Poll(ctx)
		cancel()
		if err == nil {
			task.finish(nil)
		}
	}()

	<-pollStarted
	time.Sleep(10 * time.Millisecond)
	task := newInternalTask(randomTaskInfo(), nil, commongenpb.TaskSourceHistory, "", true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	syncMatch, err := t.matcher.Offer(ctx, task)
	cancel()
	t.NoError(err)
	t.True(syncMatch)
}

func (t *MatcherTestSuite) TestRemoteSyncMatch() {
	t.testRemoteSyncMatch(commongenpb.TaskSourceHistory)
}

func (t *MatcherTestSuite) TestRemoteSyncMatchBlocking() {
	t.testRemoteSyncMatch(commongenpb.TaskSourceDbBacklog)
}

func (t *MatcherTestSuite) testRemoteSyncMatch(taskSource commongenpb.TaskSource) {
	pollSigC := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		<-pollSigC
		if taskSource == commongenpb.TaskSourceDbBacklog {
			// when task is from dbBacklog, sync match SHOULD block
			// so lets delay polling by a bit to verify that
			time.Sleep(time.Millisecond * 10)
		}
		task, err := t.matcher.Poll(ctx)
		cancel()
		if err == nil && !task.isStarted() {
			task.finish(nil)
		}
	}()

	var remotePollErr error
	var remotePollResp matchingservice.PollForDecisionTaskResponse
	t.client.EXPECT().PollForDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.PollForDecisionTaskRequest) {
			task, err := t.rootMatcher.Poll(arg0)
			if err != nil {
				remotePollErr = err
			} else {
				task.finish(nil)
				remotePollResp = matchingservice.PollForDecisionTaskResponse{
					WorkflowExecution: task.workflowExecution(),
				}
			}
		},
	).Return(&remotePollResp, remotePollErr).AnyTimes()

	task := newInternalTask(randomTaskInfo(), nil, taskSource, "", true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)

	var err error
	var remoteSyncMatch bool
	var req *matchingservice.AddDecisionTaskRequest
	t.client.EXPECT().AddDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.AddDecisionTaskRequest) {
			req = arg1
			task.forwardedFrom = req.GetForwardedFrom()
			close(pollSigC)
			if taskSource != commongenpb.TaskSourceDbBacklog {
				// when task is not from backlog, wait a bit for poller
				// to arrive first - when task is from backlog, offer
				// blocks - so we don't need to do this
				time.Sleep(10 * time.Millisecond)
			}
			remoteSyncMatch, err = t.rootMatcher.Offer(ctx, task)
		},
	).Return(&matchingservice.AddDecisionTaskResponse{}, nil)

	_, err0 := t.matcher.Offer(ctx, task)
	t.NoError(err0)
	cancel()
	t.NotNil(req)
	t.NoError(err)
	t.True(remoteSyncMatch)
	t.Equal(t.taskList.name, req.GetForwardedFrom())
	t.Equal(t.taskList.Parent(20), req.GetTaskList().GetName())
}

func (t *MatcherTestSuite) TestSyncMatchFailure() {
	task := newInternalTask(randomTaskInfo(), nil, commongenpb.TaskSourceHistory, "", true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)

	var req *matchingservice.AddDecisionTaskRequest
	t.client.EXPECT().AddDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.AddDecisionTaskRequest) {
			req = arg1
		},
	).Return(&matchingservice.AddDecisionTaskResponse{}, errMatchingHostThrottle)

	syncMatch, err := t.matcher.Offer(ctx, task)
	cancel()
	t.NotNil(req)
	t.NoError(err)
	t.False(syncMatch)
}

func (t *MatcherTestSuite) TestQueryLocalSyncMatch() {
	// force disable remote forwarding
	<-t.fwdr.AddReqTokenC()
	<-t.fwdr.PollReqTokenC()

	pollStarted := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		close(pollStarted)
		task, err := t.matcher.PollForQuery(ctx)
		cancel()
		if err == nil && task.isQuery() {
			task.finish(nil)
		}
	}()

	<-pollStarted
	time.Sleep(10 * time.Millisecond)
	task := newInternalQueryTask(uuid.New(), &matchingservice.QueryWorkflowRequest{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := t.matcher.OfferQuery(ctx, task)
	cancel()
	t.NoError(err)
	t.Nil(resp)
}

func (t *MatcherTestSuite) TestQueryRemoteSyncMatch() {
	pollSigC := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		<-pollSigC
		task, err := t.matcher.PollForQuery(ctx)
		cancel()
		if err == nil && task.isQuery() {
			task.finish(nil)
		}
	}()

	var querySet = atomic.NewBool(false)
	var remotePollErr error
	var remotePollResp matchingservice.PollForDecisionTaskResponse
	t.client.EXPECT().PollForDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.PollForDecisionTaskRequest) {
			task, err := t.rootMatcher.PollForQuery(arg0)
			if err != nil {
				remotePollErr = err
			} else if task.isQuery() {
				task.finish(nil)
				querySet.Swap(true)
				remotePollResp = matchingservice.PollForDecisionTaskResponse{
					Query: &querypb.WorkflowQuery{},
				}
			}
		},
	).Return(&remotePollResp, remotePollErr).AnyTimes()

	task := newInternalQueryTask(uuid.New(), &matchingservice.QueryWorkflowRequest{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)

	var req *matchingservice.QueryWorkflowRequest
	t.client.EXPECT().QueryWorkflow(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.QueryWorkflowRequest) {
			req = arg1
			task.forwardedFrom = req.GetForwardedFrom()
			close(pollSigC)
			time.Sleep(10 * time.Millisecond)
			t.rootMatcher.OfferQuery(ctx, task)
		},
	).Return(&matchingservice.QueryWorkflowResponse{QueryResult: []byte("answer")}, nil)

	result, err := t.matcher.OfferQuery(ctx, task)
	cancel()
	t.NotNil(req)
	t.NoError(err)
	t.NotNil(result)
	t.True(querySet.Load())
	t.Equal("answer", string(result.QueryResult))
	t.Equal(t.taskList.name, req.GetForwardedFrom())
	t.Equal(t.taskList.Parent(20), req.GetTaskList().GetName())
}

func (t *MatcherTestSuite) TestQueryRemoteSyncMatchError() {
	<-t.fwdr.PollReqTokenC()

	matched := false
	pollSigC := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		<-pollSigC
		task, err := t.matcher.PollForQuery(ctx)
		cancel()
		if err == nil && task.isQuery() {
			matched = true
			task.finish(nil)
		}
	}()

	task := newInternalQueryTask(uuid.New(), &matchingservice.QueryWorkflowRequest{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)

	var req *matchingservice.QueryWorkflowRequest
	t.client.EXPECT().QueryWorkflow(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.QueryWorkflowRequest) {
			req = arg1
			close(pollSigC)
			time.Sleep(10 * time.Millisecond)
		},
	).Return(nil, errMatchingHostThrottle)

	result, err := t.matcher.OfferQuery(ctx, task)
	cancel()
	t.NotNil(req)
	t.NoError(err)
	t.Nil(result)
	t.True(matched)
}

// todo: note from shawn, when does this case happen in production?
func (t *MatcherTestSuite) TestMustOfferLocalMatch() {
	// force disable remote forwarding
	<-t.fwdr.AddReqTokenC()
	<-t.fwdr.PollReqTokenC()

	pollStarted := make(chan struct{})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		close(pollStarted)
		task, err := t.matcher.Poll(ctx)
		cancel()
		if err == nil {
			task.finish(nil)
		}
	}()

	<-pollStarted
	time.Sleep(10 * time.Millisecond)
	task := newInternalTask(randomTaskInfo(), nil, commongenpb.TaskSourceHistory, "", false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	err := t.matcher.MustOffer(ctx, task)
	cancel()
	t.NoError(err)
}

func (t *MatcherTestSuite) TestMustOfferRemoteMatch() {
	pollSigC := make(chan struct{})

	var remotePollErr error
	var remotePollResp matchingservice.PollForDecisionTaskResponse
	t.client.EXPECT().PollForDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.PollForDecisionTaskRequest) {
			<-pollSigC
			time.Sleep(time.Millisecond * 500) // delay poll to verify that offer blocks on parent
			task, err := t.rootMatcher.Poll(arg0)
			if err != nil {
				remotePollErr = err
			} else {
				task.finish(nil)
				remotePollResp = matchingservice.PollForDecisionTaskResponse{
					WorkflowExecution: task.workflowExecution(),
				}
			}
		},
	).Return(&remotePollResp, remotePollErr).AnyTimes()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		t.matcher.Poll(ctx)
		cancel()
	}()

	taskCompleted := false
	completionFunc := func(*persistenceblobs.AllocatedTaskInfo, error) {
		taskCompleted = true
	}

	task := newInternalTask(randomTaskInfo(), completionFunc, commongenpb.TaskSourceDbBacklog, "", false)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)

	var err error
	var remoteSyncMatch bool
	var req *matchingservice.AddDecisionTaskRequest
	t.client.EXPECT().AddDecisionTask(gomock.Any(), gomock.Any()).Return(&matchingservice.AddDecisionTaskResponse{}, errMatchingHostThrottle).Times(1)
	t.client.EXPECT().AddDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.AddDecisionTaskRequest) {
			req = arg1
			task := newInternalTask(task.event.AllocatedTaskInfo, nil, commongenpb.TaskSourceDbBacklog, req.GetForwardedFrom(), true)
			close(pollSigC)
			remoteSyncMatch, err = t.rootMatcher.Offer(ctx, task)
		},
	).Return(&matchingservice.AddDecisionTaskResponse{}, nil)

	t.NoError(t.matcher.MustOffer(ctx, task))
	cancel()
	t.NotNil(req)
	t.NoError(err)
	t.True(remoteSyncMatch)
	t.True(taskCompleted)
	t.Equal(t.taskList.name, req.GetForwardedFrom())
	t.Equal(t.taskList.Parent(20), req.GetTaskList().GetName())
}

func (t *MatcherTestSuite) TestRemotePoll() {
	pollToken := <-t.fwdr.PollReqTokenC()

	var req *matchingservice.PollForDecisionTaskRequest
	t.client.EXPECT().PollForDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.PollForDecisionTaskRequest) {
			req = arg1
		},
	).Return(&matchingservice.PollForDecisionTaskResponse{}, nil)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pollToken.release()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	task, err := t.matcher.Poll(ctx)
	cancel()
	t.NoError(err)
	t.NotNil(req)
	t.NotNil(task)
	t.True(task.isStarted())
}

func (t *MatcherTestSuite) TestRemotePollForQuery() {
	pollToken := <-t.fwdr.PollReqTokenC()

	var req *matchingservice.PollForDecisionTaskRequest
	t.client.EXPECT().PollForDecisionTask(gomock.Any(), gomock.Any()).Do(
		func(arg0 context.Context, arg1 *matchingservice.PollForDecisionTaskRequest) {
			req = arg1
		},
	).Return(&matchingservice.PollForDecisionTaskResponse{}, nil)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pollToken.release()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	task, err := t.matcher.PollForQuery(ctx)
	cancel()
	t.NoError(err)
	t.NotNil(req)
	t.NotNil(task)
	t.True(task.isStarted())
}

func (t *MatcherTestSuite) newNamespaceCache() cache.NamespaceCache {
	entry := cache.NewLocalNamespaceCacheEntryForTest(
		&persistence.NamespaceInfo{Name: "test-namespace"},
		&persistence.NamespaceConfig{},
		"",
		nil)
	dc := cache.NewMockNamespaceCache(t.controller)
	dc.EXPECT().GetNamespaceByID(gomock.Any()).Return(entry, nil).AnyTimes()
	return dc
}

func randomTaskInfo() *persistenceblobs.AllocatedTaskInfo {
	rt1 := time.Date(rand.Intn(9999), time.Month(rand.Intn(12)+1), rand.Intn(28)+1, rand.Intn(24)+1, rand.Intn(60), rand.Intn(60), rand.Intn(1e9), time.UTC)
	rt2 := time.Date(rand.Intn(5000)+3000, time.Month(rand.Intn(12)+1), rand.Intn(28)+1, rand.Intn(24)+1, rand.Intn(60), rand.Intn(60), rand.Intn(1e9), time.UTC)

	return &persistenceblobs.AllocatedTaskInfo{
		Data: &persistenceblobs.TaskInfo{
			NamespaceId: uuid.NewRandom(),
			WorkflowId:  uuid.New(),
			RunId:       uuid.NewRandom(),
			ScheduleId:  rand.Int63(),
			CreatedTime: timestamp.TimestampFromTime(&rt1).ToProto(),
			Expiry:      timestamp.TimestampFromTime(&rt2).ToProto(),
		},
		TaskId: rand.Int63(),
	}
}
