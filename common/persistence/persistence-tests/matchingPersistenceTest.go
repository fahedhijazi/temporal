// Copyright (c) 2017 Uber Technologies, Inc.
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

package persistencetests

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	executionpb "go.temporal.io/temporal-proto/execution"

	"github.com/temporalio/temporal/.gen/proto/persistenceblobs"
	p "github.com/temporalio/temporal/common/persistence"
	"github.com/temporalio/temporal/common/primitives"
)

type (
	// MatchingPersistenceSuite contains matching persistence tests
	MatchingPersistenceSuite struct {
		TestBase
		// override suite.Suite.Assertions with require.Assertions; this means that s.NotNil(nil) will stop the test,
		// not merely log an error
		*require.Assertions
	}
)

// TimePrecision is needed to account for database timestamp precision.
// Cassandra only provides milliseconds timestamp precision, so we need to use tolerance when doing comparison
const TimePrecision = 2 * time.Millisecond

// SetupSuite implementation
func (s *MatchingPersistenceSuite) SetupSuite() {
	if testing.Verbose() {
		log.SetOutput(os.Stdout)
	}
}

// TearDownSuite implementation
func (s *MatchingPersistenceSuite) TearDownSuite() {
	s.TearDownWorkflowStore()
}

// SetupTest implementation
func (s *MatchingPersistenceSuite) SetupTest() {
	// Have to define our overridden assertions in the test setup. If we did it earlier, s.T() will return nil
	s.Assertions = require.New(s.T())
}

// TestCreateTask test
func (s *MatchingPersistenceSuite) TestCreateTask() {
	namespaceID := primitives.MustParseUUID("11adbd1b-f164-4ea7-b2f3-2e857a5048f1")
	workflowExecution := executionpb.WorkflowExecution{WorkflowId: "create-task-test",
		RunId: "c949447a-691a-4132-8b2a-a5b38106793c"}
	task0, err0 := s.CreateDecisionTask(namespaceID, workflowExecution, "a5b38106793c", 5)
	s.NoError(err0)
	s.NotNil(task0, "Expected non empty task identifier.")

	tasks1, err1 := s.CreateActivityTasks(namespaceID, workflowExecution, map[int64]string{
		10: "a5b38106793c"})
	s.NoError(err1)
	s.NotNil(tasks1, "Expected valid task identifiers.")
	s.Equal(1, len(tasks1), "expected single valid task identifier.")
	for _, t := range tasks1 {
		s.NotEmpty(t, "Expected non empty task identifier.")
	}

	tasks := map[int64]string{
		20: uuid.New(),
		30: uuid.New(),
		40: uuid.New(),
		50: uuid.New(),
		60: uuid.New(),
	}
	tasks2, err2 := s.CreateActivityTasks(namespaceID, workflowExecution, tasks)
	s.NoError(err2)
	s.Equal(5, len(tasks2), "expected single valid task identifier.")

	for sid, tlName := range tasks {
		resp, err := s.GetTasks(namespaceID, tlName, p.TaskListTypeActivity, 100)
		s.NoError(err)
		s.Equal(1, len(resp.Tasks))
		s.EqualValues(namespaceID, resp.Tasks[0].Data.GetNamespaceId())
		s.Equal(workflowExecution.WorkflowId, resp.Tasks[0].Data.GetWorkflowId())
		s.EqualValues(primitives.MustParseUUID(workflowExecution.RunId), resp.Tasks[0].Data.GetRunId())
		s.Equal(sid, resp.Tasks[0].Data.GetScheduleId())
		cTime, err := types.TimestampFromProto(resp.Tasks[0].Data.CreatedTime)
		s.NoError(err)
		eTime, err := types.TimestampFromProto(resp.Tasks[0].Data.Expiry)
		s.NoError(err)
		s.True(cTime.UnixNano() > 0)
		if s.TaskMgr.GetName() != "cassandra" {
			// cassandra uses TTL and expiry isn't stored as part of task state
			s.True(time.Now().Before(eTime))
			s.True(eTime.Before(time.Now().Add((defaultScheduleToStartTimeout + 1) * time.Second)))
		}
	}
}

// TestGetDecisionTasks test
func (s *MatchingPersistenceSuite) TestGetDecisionTasks() {
	namespaceID := primitives.MustParseUUID("aeac8287-527b-4b35-80a9-667cb47e7c6d")
	workflowExecution := executionpb.WorkflowExecution{WorkflowId: "get-decision-task-test",
		RunId: "db20f7e2-1a1e-40d9-9278-d8b886738e05"}
	taskList := "d8b886738e05"
	task0, err0 := s.CreateDecisionTask(namespaceID, workflowExecution, taskList, 5)
	s.NoError(err0)
	s.NotNil(task0, "Expected non empty task identifier.")

	tasks1Response, err1 := s.GetTasks(namespaceID, taskList, p.TaskListTypeDecision, 1)
	s.NoError(err1)
	s.NotNil(tasks1Response.Tasks, "expected valid list of tasks.")
	s.Equal(1, len(tasks1Response.Tasks), "Expected 1 decision task.")
	s.Equal(int64(5), tasks1Response.Tasks[0].Data.GetScheduleId())
}

// TestGetTasksWithNoMaxReadLevel test
func (s *MatchingPersistenceSuite) TestGetTasksWithNoMaxReadLevel() {
	if s.TaskMgr.GetName() == "cassandra" {
		s.T().Skip("this test is not applicable for cassandra persistence")
	}
	namespaceID := primitives.MustParseUUID("f1116985-d1f1-40e0-aba9-83344db915bc")
	workflowExecution := executionpb.WorkflowExecution{WorkflowId: "complete-decision-task-test",
		RunId: "2aa0a74e-16ee-4f27-983d-48b07ec1915d"}
	taskList := "48b07ec1915d"
	_, err0 := s.CreateActivityTasks(namespaceID, workflowExecution, map[int64]string{
		10: taskList,
		20: taskList,
		30: taskList,
		40: taskList,
		50: taskList,
	})
	s.NoError(err0)

	nTasks := 5
	firstTaskID := s.GetNextSequenceNumber() - int64(nTasks)

	testCases := []struct {
		batchSz   int
		readLevel int64
		taskIDs   []int64
	}{
		{1, -1, []int64{firstTaskID}},
		{2, firstTaskID, []int64{firstTaskID + 1, firstTaskID + 2}},
		{5, firstTaskID + 2, []int64{firstTaskID + 3, firstTaskID + 4}},
	}

	for _, tc := range testCases {
		s.Run(fmt.Sprintf("tc_%v_%v", tc.batchSz, tc.readLevel), func() {
			response, err := s.TaskMgr.GetTasks(&p.GetTasksRequest{
				NamespaceID: namespaceID,
				TaskList:    taskList,
				TaskType:    p.TaskListTypeActivity,
				BatchSize:   tc.batchSz,
				ReadLevel:   tc.readLevel,
			})
			s.NoError(err)
			s.Equal(len(tc.taskIDs), len(response.Tasks), "wrong number of tasks")
			for i := range tc.taskIDs {
				s.Equal(tc.taskIDs[i], response.Tasks[i].GetTaskId(), "wrong set of tasks")
			}
		})
	}
}

// TestCompleteDecisionTask test
func (s *MatchingPersistenceSuite) TestCompleteDecisionTask() {
	namespaceID := primitives.MustParseUUID("f1116985-d1f1-40e0-aba9-83344db915bc")
	workflowExecution := executionpb.WorkflowExecution{WorkflowId: "complete-decision-task-test",
		RunId: "2aa0a74e-16ee-4f27-983d-48b07ec1915d"}
	taskList := "48b07ec1915d"
	tasks0, err0 := s.CreateActivityTasks(namespaceID, workflowExecution, map[int64]string{
		10: taskList,
		20: taskList,
		30: taskList,
		40: taskList,
		50: taskList,
	})
	s.NoError(err0)
	s.NotNil(tasks0, "Expected non empty task identifier.")
	s.Equal(5, len(tasks0), "expected 5 valid task identifier.")
	for _, t := range tasks0 {
		s.NotEmpty(t, "Expected non empty task identifier.")
	}

	tasksWithID1Response, err1 := s.GetTasks(namespaceID, taskList, p.TaskListTypeActivity, 5)

	s.NoError(err1)
	tasksWithID1 := tasksWithID1Response.Tasks
	s.NotNil(tasksWithID1, "expected valid list of tasks.")

	s.Equal(5, len(tasksWithID1), "Expected 5 activity tasks.")
	for _, t := range tasksWithID1 {
		s.EqualValues(namespaceID, t.Data.GetNamespaceId())
		s.Equal(workflowExecution.WorkflowId, t.Data.GetWorkflowId())
		s.EqualValues(primitives.MustParseUUID(workflowExecution.RunId), t.Data.GetRunId())
		s.True(t.GetTaskId() > 0)

		err2 := s.CompleteTask(namespaceID, taskList, p.TaskListTypeActivity, t.GetTaskId())
		s.NoError(err2)
	}
}

// TestCompleteTasksLessThan test
func (s *MatchingPersistenceSuite) TestCompleteTasksLessThan() {
	namespaceID := primitives.UUID(uuid.NewRandom())
	taskList := "range-complete-task-tl0"
	wfExec := executionpb.WorkflowExecution{
		WorkflowId: "range-complete-task-test",
		RunId:      uuid.New(),
	}
	_, err := s.CreateActivityTasks(namespaceID, wfExec, map[int64]string{
		10: taskList,
		20: taskList,
		30: taskList,
		40: taskList,
		50: taskList,
		60: taskList,
	})
	s.NoError(err)

	resp, err := s.GetTasks(namespaceID, taskList, p.TaskListTypeActivity, 10)
	s.NoError(err)
	s.NotNil(resp.Tasks)
	s.Equal(6, len(resp.Tasks), "getTasks returned wrong number of tasks")

	tasks := resp.Tasks

	testCases := []struct {
		taskID int64
		limit  int
		output []int64
	}{
		{
			taskID: tasks[5].GetTaskId(),
			limit:  1,
			output: []int64{tasks[1].GetTaskId(), tasks[2].GetTaskId(), tasks[3].GetTaskId(), tasks[4].GetTaskId(), tasks[5].GetTaskId()},
		},
		{
			taskID: tasks[5].GetTaskId(),
			limit:  2,
			output: []int64{tasks[3].GetTaskId(), tasks[4].GetTaskId(), tasks[5].GetTaskId()},
		},
		{
			taskID: tasks[5].GetTaskId(),
			limit:  10,
			output: []int64{},
		},
	}

	remaining := len(resp.Tasks)
	req := &p.CompleteTasksLessThanRequest{NamespaceID: namespaceID, TaskListName: taskList, TaskType: p.TaskListTypeActivity, Limit: 1}

	for _, tc := range testCases {
		req.TaskID = tc.taskID
		req.Limit = tc.limit
		nRows, err := s.TaskMgr.CompleteTasksLessThan(req)
		s.NoError(err)
		resp, err := s.GetTasks(namespaceID, taskList, p.TaskListTypeActivity, 10)
		s.NoError(err)
		if nRows == p.UnknownNumRowsAffected {
			s.Equal(0, len(resp.Tasks), "expected all tasks to be deleted")
			break
		}
		s.Equal(remaining-len(tc.output), nRows, "expected only LIMIT number of rows to be deleted")
		s.Equal(len(tc.output), len(resp.Tasks), "rangeCompleteTask deleted wrong set of tasks")
		for i := range tc.output {
			s.Equal(tc.output[i], resp.Tasks[i].GetTaskId())
		}
		remaining = len(tc.output)
	}
}

// TestLeaseAndUpdateTaskList test
func (s *MatchingPersistenceSuite) TestLeaseAndUpdateTaskList() {
	namespaceID := primitives.MustParseUUID("00136543-72ad-4615-b7e9-44bca9775b45")
	taskList := "aaaaaaa"
	leaseTime := time.Now()
	response, err := s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
		NamespaceID: namespaceID,
		TaskList:    taskList,
		TaskType:    p.TaskListTypeActivity,
	})
	s.NoError(err)
	tli := response.TaskListInfo
	s.EqualValues(1, tli.RangeID)
	s.EqualValues(0, tli.Data.AckLevel)
	lu, err := types.TimestampFromProto(tli.Data.LastUpdated)
	s.NoError(err)
	s.True(lu.After(leaseTime) || lu.Equal(leaseTime))

	leaseTime = time.Now()
	response, err = s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
		NamespaceID: namespaceID,
		TaskList:    taskList,
		TaskType:    p.TaskListTypeActivity,
	})
	s.NoError(err)
	tli = response.TaskListInfo
	s.NotNil(tli)
	s.EqualValues(2, tli.RangeID)
	s.EqualValues(0, tli.Data.AckLevel)
	lu2, err := types.TimestampFromProto(tli.Data.LastUpdated)
	s.NoError(err)
	s.True(lu2.After(leaseTime) || lu2.Equal(leaseTime))

	response, err = s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
		NamespaceID: namespaceID,
		TaskList:    taskList,
		TaskType:    p.TaskListTypeActivity,
		RangeID:     1,
	})
	s.Error(err)
	_, ok := err.(*p.ConditionFailedError)
	s.True(ok)

	taskListInfo := &persistenceblobs.TaskListInfo{
		NamespaceId: namespaceID,
		Name:        taskList,
		TaskType:    p.TaskListTypeActivity,
		AckLevel:    0,
		Kind:        p.TaskListKindNormal,
	}

	_, err = s.TaskMgr.UpdateTaskList(&p.UpdateTaskListRequest{
		TaskListInfo: taskListInfo,
		RangeID:      2,
	})
	s.NoError(err)

	_, err = s.TaskMgr.UpdateTaskList(&p.UpdateTaskListRequest{
		TaskListInfo: taskListInfo,
		RangeID:      3,
	})
	s.Error(err)
}

// TestLeaseAndUpdateTaskListSticky test
func (s *MatchingPersistenceSuite) TestLeaseAndUpdateTaskListSticky() {
	namespaceID := primitives.UUID(uuid.NewRandom())
	taskList := "aaaaaaa"
	response, err := s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
		NamespaceID:  namespaceID,
		TaskList:     taskList,
		TaskType:     p.TaskListTypeDecision,
		TaskListKind: p.TaskListKindSticky,
	})
	s.NoError(err)
	tli := response.TaskListInfo
	s.EqualValues(1, tli.RangeID)
	s.EqualValues(0, tli.Data.AckLevel)
	s.EqualValues(p.TaskListKindSticky, tli.Data.Kind)

	taskListInfo := &persistenceblobs.TaskListInfo{
		NamespaceId: namespaceID,
		Name:        taskList,
		TaskType:    p.TaskListTypeDecision,
		AckLevel:    0,
		Kind:        p.TaskListKindSticky,
	}
	_, err = s.TaskMgr.UpdateTaskList(&p.UpdateTaskListRequest{
		TaskListInfo: taskListInfo,
		RangeID:      2,
	})
	s.NoError(err) // because update with ttl doesn't check rangeID
}

func (s *MatchingPersistenceSuite) deleteAllTaskList() {
	var nextPageToken []byte
	for {
		resp, err := s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10, PageToken: nextPageToken})
		s.NoError(err)
		for _, i := range resp.Items {
			it := i.Data
			err = s.TaskMgr.DeleteTaskList(&p.DeleteTaskListRequest{
				TaskList: &p.TaskListKey{
					NamespaceID: it.GetNamespaceId(),
					Name:        it.Name,
					TaskType:    it.TaskType,
				},
				RangeID: i.RangeID,
			})
			s.NoError(err)
		}
		nextPageToken = resp.NextPageToken
		if nextPageToken == nil {
			break
		}
	}
}

// TestListWithOneTaskList test
func (s *MatchingPersistenceSuite) TestListWithOneTaskList() {
	if s.TaskMgr.GetName() == "cassandra" {
		s.T().Skip("ListTaskList API is currently not supported in cassandra")
	}
	s.deleteAllTaskList()
	resp, err := s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10})
	s.NoError(err)
	s.Nil(resp.NextPageToken)
	s.Equal(0, len(resp.Items))

	rangeID := int64(0)
	ackLevel := int64(0)
	namespaceID := primitives.UUID(uuid.NewRandom())
	for i := 0; i < 10; i++ {
		rangeID++
		updatedTime := time.Now().UTC()
		_, err := s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
			NamespaceID:  namespaceID,
			TaskList:     "list-task-list-test-tl0",
			TaskType:     p.TaskListTypeActivity,
			TaskListKind: p.TaskListKindSticky,
		})
		s.NoError(err)

		resp, err := s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10})
		s.NoError(err)

		s.Equal(1, len(resp.Items))
		s.EqualValues(namespaceID, resp.Items[0].Data.GetNamespaceId())
		s.Equal("list-task-list-test-tl0", resp.Items[0].Data.Name)
		s.Equal(p.TaskListTypeActivity, resp.Items[0].Data.TaskType)
		s.EqualValues(p.TaskListKindSticky, resp.Items[0].Data.Kind)
		s.Equal(rangeID, resp.Items[0].RangeID)
		s.Equal(ackLevel, resp.Items[0].Data.AckLevel)
		lu0, err := types.TimestampFromProto(resp.Items[0].Data.LastUpdated)
		s.NoError(err)
		s.True(lu0.After(updatedTime) || lu0.Equal(updatedTime))

		ackLevel++
		updateTL := resp.Items[0].Data
		updateTL.AckLevel = ackLevel
		updatedTime = time.Now()
		_, err = s.TaskMgr.UpdateTaskList(&p.UpdateTaskListRequest{
			TaskListInfo: updateTL,
			RangeID:      rangeID,
		})
		s.NoError(err)

		resp, err = s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10})
		s.NoError(err)
		s.Equal(1, len(resp.Items))
		lu0, err = types.TimestampFromProto(resp.Items[0].Data.LastUpdated)
		s.NoError(err)
		s.True(lu0.After(updatedTime) || lu0.Equal(updatedTime))
	}

	s.deleteAllTaskList()
}

// TestListWithMultipleTaskList test
func (s *MatchingPersistenceSuite) TestListWithMultipleTaskList() {
	if s.TaskMgr.GetName() == "cassandra" {
		s.T().Skip("ListTaskList API is currently not supported in cassandra")
	}
	s.deleteAllTaskList()
	namespaceID := uuid.New()
	tlNames := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("test-list-with-multiple-%v", i)
		_, err := s.TaskMgr.LeaseTaskList(&p.LeaseTaskListRequest{
			NamespaceID:  primitives.MustParseUUID(namespaceID),
			TaskList:     name,
			TaskType:     p.TaskListTypeActivity,
			TaskListKind: p.TaskListKindNormal,
		})
		s.NoError(err)
		tlNames[name] = struct{}{}
		listedNames := make(map[string]struct{})
		var nextPageToken []byte
		for {
			resp, err := s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10, PageToken: nextPageToken})
			s.NoError(err)
			for _, i := range resp.Items {
				it := i.Data
				s.EqualValues(primitives.MustParseUUID(namespaceID), it.GetNamespaceId())
				s.Equal(p.TaskListTypeActivity, it.TaskType)
				s.Equal(p.TaskListKindNormal, it.Kind)
				_, ok := listedNames[it.Name]
				s.False(ok, "list API returns duplicate entries - have: %+v got:%v", listedNames, it.Name)
				listedNames[it.Name] = struct{}{}
			}
			nextPageToken = resp.NextPageToken
			if nextPageToken == nil {
				break
			}
		}
		s.Equal(tlNames, listedNames, "list API returned wrong set of task list names")
	}
	s.deleteAllTaskList()
	resp, err := s.TaskMgr.ListTaskList(&p.ListTaskListRequest{PageSize: 10})
	s.NoError(err)
	s.Nil(resp.NextPageToken)
	s.Equal(0, len(resp.Items))
}
