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

package history

import (
	"fmt"

	"github.com/pborman/uuid"
	commonpb "go.temporal.io/temporal-proto/common"
	decisionpb "go.temporal.io/temporal-proto/decision"
	eventpb "go.temporal.io/temporal-proto/event"
	"go.temporal.io/temporal-proto/serviceerror"

	"github.com/temporalio/temporal/common"
	"github.com/temporalio/temporal/common/backoff"
	"github.com/temporalio/temporal/common/cache"
	"github.com/temporalio/temporal/common/log"
	"github.com/temporalio/temporal/common/log/tag"
	"github.com/temporalio/temporal/common/metrics"
)

type (
	decisionAttrValidationFn func() error

	decisionTaskHandlerImpl struct {
		identity                string
		decisionTaskCompletedID int64
		namespaceEntry          *cache.NamespaceCacheEntry

		// internal state
		hasUnhandledEventsBeforeDecisions bool
		failDecisionInfo                  *failDecisionInfo
		activityNotStartedCancelled       bool
		continueAsNewBuilder              mutableState
		stopProcessing                    bool // should stop processing any more decisions
		mutableState                      mutableState

		// validation
		attrValidator    *decisionAttrValidator
		sizeLimitChecker *workflowSizeChecker

		logger         log.Logger
		namespaceCache cache.NamespaceCache
		metricsClient  metrics.Client
		config         *Config
	}

	failDecisionInfo struct {
		cause   eventpb.DecisionTaskFailedCause
		message string
	}
)

func newDecisionTaskHandler(
	identity string,
	decisionTaskCompletedID int64,
	namespaceEntry *cache.NamespaceCacheEntry,
	mutableState mutableState,
	attrValidator *decisionAttrValidator,
	sizeLimitChecker *workflowSizeChecker,
	logger log.Logger,
	namespaceCache cache.NamespaceCache,
	metricsClient metrics.Client,
	config *Config,
) *decisionTaskHandlerImpl {

	return &decisionTaskHandlerImpl{
		identity:                identity,
		decisionTaskCompletedID: decisionTaskCompletedID,
		namespaceEntry:          namespaceEntry,

		// internal state
		hasUnhandledEventsBeforeDecisions: mutableState.HasBufferedEvents(),
		failDecisionInfo:                  nil,
		activityNotStartedCancelled:       false,
		continueAsNewBuilder:              nil,
		stopProcessing:                    false,
		mutableState:                      mutableState,

		// validation
		attrValidator:    attrValidator,
		sizeLimitChecker: sizeLimitChecker,

		logger:         logger,
		namespaceCache: namespaceCache,
		metricsClient:  metricsClient,
		config:         config,
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisions(
	executionContext []byte,
	decisions []*decisionpb.Decision,
) error {

	// overall workflow size / count check
	failWorkflow, err := handler.sizeLimitChecker.failWorkflowSizeExceedsLimit()
	if err != nil || failWorkflow {
		return err
	}

	for _, decision := range decisions {

		err = handler.handleDecision(decision)
		if err != nil || handler.stopProcessing {
			return err
		}
	}

	handler.mutableState.GetExecutionInfo().ExecutionContext = executionContext
	return nil
}

func (handler *decisionTaskHandlerImpl) handleDecision(decision *decisionpb.Decision) error {
	switch decision.GetDecisionType() {
	case decisionpb.DecisionTypeScheduleActivityTask:
		return handler.handleDecisionScheduleActivity(decision.GetScheduleActivityTaskDecisionAttributes())

	case decisionpb.DecisionTypeCompleteWorkflowExecution:
		return handler.handleDecisionCompleteWorkflow(decision.GetCompleteWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeFailWorkflowExecution:
		return handler.handleDecisionFailWorkflow(decision.GetFailWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeCancelWorkflowExecution:
		return handler.handleDecisionCancelWorkflow(decision.GetCancelWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeStartTimer:
		return handler.handleDecisionStartTimer(decision.GetStartTimerDecisionAttributes())

	case decisionpb.DecisionTypeRequestCancelActivityTask:
		return handler.handleDecisionRequestCancelActivity(decision.GetRequestCancelActivityTaskDecisionAttributes())

	case decisionpb.DecisionTypeCancelTimer:
		return handler.handleDecisionCancelTimer(decision.GetCancelTimerDecisionAttributes())

	case decisionpb.DecisionTypeRecordMarker:
		return handler.handleDecisionRecordMarker(decision.GetRecordMarkerDecisionAttributes())

	case decisionpb.DecisionTypeRequestCancelExternalWorkflowExecution:
		return handler.handleDecisionRequestCancelExternalWorkflow(decision.GetRequestCancelExternalWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeSignalExternalWorkflowExecution:
		return handler.handleDecisionSignalExternalWorkflow(decision.GetSignalExternalWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeContinueAsNewWorkflowExecution:
		return handler.handleDecisionContinueAsNewWorkflow(decision.GetContinueAsNewWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeStartChildWorkflowExecution:
		return handler.handleDecisionStartChildWorkflow(decision.GetStartChildWorkflowExecutionDecisionAttributes())

	case decisionpb.DecisionTypeUpsertWorkflowSearchAttributes:
		return handler.handleDecisionUpsertWorkflowSearchAttributes(decision.GetUpsertWorkflowSearchAttributesDecisionAttributes())

	default:
		return serviceerror.NewInvalidArgument(fmt.Sprintf("Unknown decision type: %v", decision.GetDecisionType()))
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisionScheduleActivity(
	attr *decisionpb.ScheduleActivityTaskDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeScheduleActivityCounter,
	)

	executionInfo := handler.mutableState.GetExecutionInfo()
	namespaceID := executionInfo.NamespaceID
	targetNamespaceID := namespaceID
	if attr.GetNamespace() != "" {
		targetNamespaceEntry, err := handler.namespaceCache.GetNamespace(attr.GetNamespace())
		if err != nil {
			return serviceerror.NewInternal(fmt.Sprintf("Unable to schedule activity across namespace %v.", attr.GetNamespace()))
		}
		targetNamespaceID = targetNamespaceEntry.GetInfo().ID
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateActivityScheduleAttributes(
				namespaceID,
				targetNamespaceID,
				attr,
				executionInfo.WorkflowTimeout,
			)
		},
		eventpb.DecisionTaskFailedCauseBadScheduleActivityAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Input,
		"ScheduleActivityTaskDecisionAttributes.Input exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	_, _, err = handler.mutableState.AddActivityTaskScheduledEvent(handler.decisionTaskCompletedID, attr)
	switch err.(type) {
	case nil:
		return nil
	case *serviceerror.InvalidArgument:
		return handler.handlerFailDecision(
			eventpb.DecisionTaskFailedCauseScheduleActivityDuplicateId, "",
		)
	default:
		return err
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisionRequestCancelActivity(
	attr *decisionpb.RequestCancelActivityTaskDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeCancelActivityCounter,
	)

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateActivityCancelAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadRequestCancelActivityAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	activityID := attr.GetActivityId()
	actCancelReqEvent, ai, err := handler.mutableState.AddActivityTaskCancelRequestedEvent(
		handler.decisionTaskCompletedID,
		activityID,
		handler.identity,
	)
	switch err.(type) {
	case nil:
		if ai.StartedID == common.EmptyEventID {
			// We haven't started the activity yet, we can cancel the activity right away and
			// schedule a decision task to ensure the workflow makes progress.
			_, err = handler.mutableState.AddActivityTaskCanceledEvent(
				ai.ScheduleID,
				ai.StartedID,
				actCancelReqEvent.GetEventId(),
				[]byte(activityCancellationMsgActivityNotStarted),
				handler.identity,
			)
			if err != nil {
				return err
			}
			handler.activityNotStartedCancelled = true
		}
		return nil
	case *serviceerror.InvalidArgument:
		_, err = handler.mutableState.AddRequestCancelActivityTaskFailedEvent(
			handler.decisionTaskCompletedID,
			activityID,
			activityCancellationMsgActivityIDUnknown,
		)
		return err
	default:
		return err
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisionStartTimer(
	attr *decisionpb.StartTimerDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeStartTimerCounter,
	)

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateTimerScheduleAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadStartTimerAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	_, _, err := handler.mutableState.AddTimerStartedEvent(handler.decisionTaskCompletedID, attr)
	switch err.(type) {
	case nil:
		return nil
	case *serviceerror.InvalidArgument:
		return handler.handlerFailDecision(
			eventpb.DecisionTaskFailedCauseStartTimerDuplicateId, "",
		)
	default:
		return err
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisionCompleteWorkflow(
	attr *decisionpb.CompleteWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeCompleteWorkflowCounter,
	)

	if handler.hasUnhandledEventsBeforeDecisions {
		return handler.handlerFailDecision(eventpb.DecisionTaskFailedCauseUnhandledDecision, "")
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateCompleteWorkflowExecutionAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadCompleteWorkflowExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Result,
		"CompleteWorkflowExecutionDecisionAttributes.Result exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	// If the decision has more than one completion event than just pick the first one
	if !handler.mutableState.IsWorkflowExecutionRunning() {
		handler.metricsClient.IncCounter(
			metrics.HistoryRespondDecisionTaskCompletedScope,
			metrics.MultipleCompletionDecisionsCounter,
		)
		handler.logger.Warn(
			"Multiple completion decisions",
			tag.WorkflowDecisionType(int64(decisionpb.DecisionTypeCompleteWorkflowExecution)),
			tag.ErrorTypeMultipleCompletionDecisions,
		)
		return nil
	}

	// check if this is a cron workflow
	cronBackoff, err := handler.mutableState.GetCronBackoffDuration()
	if err != nil {
		handler.stopProcessing = true
		return err
	}
	if cronBackoff == backoff.NoBackoff {
		// not cron, so complete this workflow execution
		if _, err := handler.mutableState.AddCompletedWorkflowEvent(handler.decisionTaskCompletedID, attr); err != nil {
			return serviceerror.NewInternal("Unable to add complete workflow event.")
		}
		return nil
	}

	// this is a cron workflow
	startEvent, err := handler.mutableState.GetStartEvent()
	if err != nil {
		return err
	}
	startAttributes := startEvent.GetWorkflowExecutionStartedEventAttributes()
	return handler.retryCronContinueAsNew(
		startAttributes,
		int32(cronBackoff.Seconds()),
		commonpb.ContinueAsNewInitiatorCronSchedule,
		"",
		nil,
		attr.Result,
	)
}

func (handler *decisionTaskHandlerImpl) handleDecisionFailWorkflow(
	attr *decisionpb.FailWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeFailWorkflowCounter,
	)

	if handler.hasUnhandledEventsBeforeDecisions {
		return handler.handlerFailDecision(eventpb.DecisionTaskFailedCauseUnhandledDecision, "")
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateFailWorkflowExecutionAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadFailWorkflowExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Details,
		"FailWorkflowExecutionDecisionAttributes.Details exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	// If the decision has more than one completion event than just pick the first one
	if !handler.mutableState.IsWorkflowExecutionRunning() {
		handler.metricsClient.IncCounter(
			metrics.HistoryRespondDecisionTaskCompletedScope,
			metrics.MultipleCompletionDecisionsCounter,
		)
		handler.logger.Warn(
			"Multiple completion decisions",
			tag.WorkflowDecisionType(int64(decisionpb.DecisionTypeFailWorkflowExecution)),
			tag.ErrorTypeMultipleCompletionDecisions,
		)
		return nil
	}

	// below will check whether to do continue as new based on backoff & backoff or cron
	backoffInterval := handler.mutableState.GetRetryBackoffDuration(attr.GetReason())
	continueAsNewInitiator := commonpb.ContinueAsNewInitiatorRetryPolicy
	// first check the backoff retry
	if backoffInterval == backoff.NoBackoff {
		// if no backoff retry, set the backoffInterval using cron schedule
		backoffInterval, err = handler.mutableState.GetCronBackoffDuration()
		if err != nil {
			handler.stopProcessing = true
			return err
		}
		continueAsNewInitiator = commonpb.ContinueAsNewInitiatorCronSchedule
	}
	// second check the backoff / cron schedule
	if backoffInterval == backoff.NoBackoff {
		// no retry or cron
		if _, err := handler.mutableState.AddFailWorkflowEvent(handler.decisionTaskCompletedID, attr); err != nil {
			return err
		}
		return nil
	}

	// this is a cron / backoff workflow
	startEvent, err := handler.mutableState.GetStartEvent()
	if err != nil {
		return err
	}
	startAttributes := startEvent.GetWorkflowExecutionStartedEventAttributes()
	return handler.retryCronContinueAsNew(
		startAttributes,
		int32(backoffInterval.Seconds()),
		continueAsNewInitiator,
		attr.Reason,
		attr.Details,
		startAttributes.LastCompletionResult,
	)
}

func (handler *decisionTaskHandlerImpl) handleDecisionCancelTimer(
	attr *decisionpb.CancelTimerDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeCancelTimerCounter,
	)

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateTimerCancelAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadCancelTimerAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	_, err := handler.mutableState.AddTimerCanceledEvent(
		handler.decisionTaskCompletedID,
		attr,
		handler.identity)
	switch err.(type) {
	case nil:
		// timer deletion is a success, we may have deleted a fired timer in
		// which case we should reset hasBufferedEvents
		// TODO deletion of timer fired event refreshing hasUnhandledEventsBeforeDecisions
		//  is not entirely correct, since during these decisions processing, new event may appear
		handler.hasUnhandledEventsBeforeDecisions = handler.mutableState.HasBufferedEvents()
		return nil
	case *serviceerror.InvalidArgument:
		_, err = handler.mutableState.AddCancelTimerFailedEvent(
			handler.decisionTaskCompletedID,
			attr,
			handler.identity,
		)
		return err
	default:
		return err
	}
}

func (handler *decisionTaskHandlerImpl) handleDecisionCancelWorkflow(
	attr *decisionpb.CancelWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeCancelWorkflowCounter)

	if handler.hasUnhandledEventsBeforeDecisions {
		return handler.handlerFailDecision(eventpb.DecisionTaskFailedCauseUnhandledDecision, "")
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateCancelWorkflowExecutionAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadCancelWorkflowExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	// If the decision has more than one completion event than just pick the first one
	if !handler.mutableState.IsWorkflowExecutionRunning() {
		handler.metricsClient.IncCounter(
			metrics.HistoryRespondDecisionTaskCompletedScope,
			metrics.MultipleCompletionDecisionsCounter,
		)
		handler.logger.Warn(
			"Multiple completion decisions",
			tag.WorkflowDecisionType(int64(decisionpb.DecisionTypeCancelWorkflowExecution)),
			tag.ErrorTypeMultipleCompletionDecisions,
		)
		return nil
	}

	_, err := handler.mutableState.AddWorkflowExecutionCanceledEvent(handler.decisionTaskCompletedID, attr)
	return err
}

func (handler *decisionTaskHandlerImpl) handleDecisionRequestCancelExternalWorkflow(
	attr *decisionpb.RequestCancelExternalWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeCancelExternalWorkflowCounter,
	)

	executionInfo := handler.mutableState.GetExecutionInfo()
	namespaceID := executionInfo.NamespaceID
	targetNamespaceID := namespaceID
	if attr.GetNamespace() != "" {
		targetNamespaceEntry, err := handler.namespaceCache.GetNamespace(attr.GetNamespace())
		if err != nil {
			return serviceerror.NewInternal(fmt.Sprintf("Unable to cancel workflow across namespace: %v.", attr.GetNamespace()))
		}
		targetNamespaceID = targetNamespaceEntry.GetInfo().ID
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateCancelExternalWorkflowExecutionAttributes(
				namespaceID,
				targetNamespaceID,
				attr,
			)
		},
		eventpb.DecisionTaskFailedCauseBadRequestCancelExternalWorkflowExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	cancelRequestID := uuid.New()
	_, _, err := handler.mutableState.AddRequestCancelExternalWorkflowExecutionInitiatedEvent(
		handler.decisionTaskCompletedID, cancelRequestID, attr,
	)
	return err
}

func (handler *decisionTaskHandlerImpl) handleDecisionRecordMarker(
	attr *decisionpb.RecordMarkerDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeRecordMarkerCounter,
	)

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateRecordMarkerAttributes(attr)
		},
		eventpb.DecisionTaskFailedCauseBadRecordMarkerAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Details,
		"RecordMarkerDecisionAttributes.Details exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	_, err = handler.mutableState.AddRecordMarkerEvent(handler.decisionTaskCompletedID, attr)
	return err
}

func (handler *decisionTaskHandlerImpl) handleDecisionContinueAsNewWorkflow(
	attr *decisionpb.ContinueAsNewWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeContinueAsNewCounter,
	)

	if handler.hasUnhandledEventsBeforeDecisions {
		return handler.handlerFailDecision(eventpb.DecisionTaskFailedCauseUnhandledDecision, "")
	}

	executionInfo := handler.mutableState.GetExecutionInfo()

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateContinueAsNewWorkflowExecutionAttributes(
				attr,
				executionInfo,
			)
		},
		eventpb.DecisionTaskFailedCauseBadContinueAsNewAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Input,
		"ContinueAsNewWorkflowExecutionDecisionAttributes. Input exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	// If the decision has more than one completion event than just pick the first one
	if !handler.mutableState.IsWorkflowExecutionRunning() {
		handler.metricsClient.IncCounter(
			metrics.HistoryRespondDecisionTaskCompletedScope,
			metrics.MultipleCompletionDecisionsCounter,
		)
		handler.logger.Warn(
			"Multiple completion decisions",
			tag.WorkflowDecisionType(int64(decisionpb.DecisionTypeContinueAsNewWorkflowExecution)),
			tag.ErrorTypeMultipleCompletionDecisions,
		)
		return nil
	}

	// Extract parentNamespace so it can be passed down to next run of workflow execution
	var parentNamespace string
	if handler.mutableState.HasParentExecution() {
		parentNamespaceID := executionInfo.ParentNamespaceID
		parentNamespaceEntry, err := handler.namespaceCache.GetNamespaceByID(parentNamespaceID)
		if err != nil {
			return err
		}
		parentNamespace = parentNamespaceEntry.GetInfo().Name
	}

	_, newStateBuilder, err := handler.mutableState.AddContinueAsNewEvent(
		handler.decisionTaskCompletedID,
		handler.decisionTaskCompletedID,
		parentNamespace,
		attr,
	)
	if err != nil {
		return err
	}

	handler.continueAsNewBuilder = newStateBuilder
	return nil
}

func (handler *decisionTaskHandlerImpl) handleDecisionStartChildWorkflow(
	attr *decisionpb.StartChildWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeChildWorkflowCounter,
	)

	executionInfo := handler.mutableState.GetExecutionInfo()
	namespaceID := executionInfo.NamespaceID
	targetNamespaceID := namespaceID
	if attr.GetNamespace() != "" {
		targetNamespaceEntry, err := handler.namespaceCache.GetNamespace(attr.GetNamespace())
		if err != nil {
			return serviceerror.NewInternal(fmt.Sprintf("Unable to schedule child execution across namespace %v.", attr.GetNamespace()))
		}
		targetNamespaceID = targetNamespaceEntry.GetInfo().ID
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateStartChildExecutionAttributes(
				namespaceID,
				targetNamespaceID,
				attr,
				executionInfo,
			)
		},
		eventpb.DecisionTaskFailedCauseBadStartChildExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Input,
		"StartChildWorkflowExecutionDecisionAttributes.Input exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	enabled := handler.config.EnableParentClosePolicy(handler.namespaceEntry.GetInfo().Name)
	if !enabled {
		attr.ParentClosePolicy = commonpb.ParentClosePolicyAbandon
	}

	requestID := uuid.New()
	_, _, err = handler.mutableState.AddStartChildWorkflowExecutionInitiatedEvent(
		handler.decisionTaskCompletedID, requestID, attr,
	)
	return err
}

func (handler *decisionTaskHandlerImpl) handleDecisionSignalExternalWorkflow(
	attr *decisionpb.SignalExternalWorkflowExecutionDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeSignalExternalWorkflowCounter,
	)

	executionInfo := handler.mutableState.GetExecutionInfo()
	namespaceID := executionInfo.NamespaceID
	targetNamespaceID := namespaceID
	if attr.GetNamespace() != "" {
		targetNamespaceEntry, err := handler.namespaceCache.GetNamespace(attr.GetNamespace())
		if err != nil {
			return serviceerror.NewInternal(fmt.Sprintf("Unable to signal workflow across namespace: %v.", attr.GetNamespace()))
		}
		targetNamespaceID = targetNamespaceEntry.GetInfo().ID
	}

	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateSignalExternalWorkflowExecutionAttributes(
				namespaceID,
				targetNamespaceID,
				attr,
			)
		},
		eventpb.DecisionTaskFailedCauseBadSignalWorkflowExecutionAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		attr.Input,
		"SignalExternalWorkflowExecutionDecisionAttributes.Input exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	signalRequestID := uuid.New() // for deduplicate
	_, _, err = handler.mutableState.AddSignalExternalWorkflowExecutionInitiatedEvent(
		handler.decisionTaskCompletedID, signalRequestID, attr,
	)
	return err
}

func (handler *decisionTaskHandlerImpl) handleDecisionUpsertWorkflowSearchAttributes(
	attr *decisionpb.UpsertWorkflowSearchAttributesDecisionAttributes,
) error {

	handler.metricsClient.IncCounter(
		metrics.HistoryRespondDecisionTaskCompletedScope,
		metrics.DecisionTypeUpsertWorkflowSearchAttributesCounter,
	)

	// get namespace name
	executionInfo := handler.mutableState.GetExecutionInfo()
	namespaceID := executionInfo.NamespaceID
	namespaceEntry, err := handler.namespaceCache.GetNamespaceByID(namespaceID)
	if err != nil {
		return serviceerror.NewInternal(fmt.Sprintf("Unable to get namespace for namespaceID: %v.", namespaceID))
	}
	namespace := namespaceEntry.GetInfo().Name

	// valid search attributes for upsert
	if err := handler.validateDecisionAttr(
		func() error {
			return handler.attrValidator.validateUpsertWorkflowSearchAttributes(
				namespace,
				attr,
			)
		},
		eventpb.DecisionTaskFailedCauseBadSearchAttributes,
	); err != nil || handler.stopProcessing {
		return err
	}

	// blob size limit check
	failWorkflow, err := handler.sizeLimitChecker.failWorkflowIfBlobSizeExceedsLimit(
		convertSearchAttributesToByteArray(attr.GetSearchAttributes().GetIndexedFields()),
		"UpsertWorkflowSearchAttributesDecisionAttributes exceeds size limit.",
	)
	if err != nil || failWorkflow {
		handler.stopProcessing = true
		return err
	}

	_, err = handler.mutableState.AddUpsertWorkflowSearchAttributesEvent(
		handler.decisionTaskCompletedID, attr,
	)
	return err
}

func convertSearchAttributesToByteArray(fields map[string][]byte) []byte {
	result := make([]byte, 0)

	for k, v := range fields {
		result = append(result, []byte(k)...)
		result = append(result, v...)
	}
	return result
}

func (handler *decisionTaskHandlerImpl) retryCronContinueAsNew(
	attr *eventpb.WorkflowExecutionStartedEventAttributes,
	backoffInterval int32,
	continueAsNewIter commonpb.ContinueAsNewInitiator,
	failureReason string,
	failureDetails []byte,
	lastCompletionResult []byte,
) error {

	continueAsNewAttributes := &decisionpb.ContinueAsNewWorkflowExecutionDecisionAttributes{
		WorkflowType:                        attr.WorkflowType,
		TaskList:                            attr.TaskList,
		RetryPolicy:                         attr.RetryPolicy,
		Input:                               attr.Input,
		ExecutionStartToCloseTimeoutSeconds: attr.ExecutionStartToCloseTimeoutSeconds,
		TaskStartToCloseTimeoutSeconds:      attr.TaskStartToCloseTimeoutSeconds,
		CronSchedule:                        attr.CronSchedule,
		BackoffStartIntervalInSeconds:       backoffInterval,
		Initiator:                           continueAsNewIter,
		FailureReason:                       failureReason,
		FailureDetails:                      failureDetails,
		LastCompletionResult:                lastCompletionResult,
		Header:                              attr.Header,
		Memo:                                attr.Memo,
		SearchAttributes:                    attr.SearchAttributes,
	}

	_, newStateBuilder, err := handler.mutableState.AddContinueAsNewEvent(
		handler.decisionTaskCompletedID,
		handler.decisionTaskCompletedID,
		attr.GetParentWorkflowNamespace(),
		continueAsNewAttributes,
	)
	if err != nil {
		return err
	}

	handler.continueAsNewBuilder = newStateBuilder
	return nil
}

func (handler *decisionTaskHandlerImpl) validateDecisionAttr(
	validationFn decisionAttrValidationFn,
	failedCause eventpb.DecisionTaskFailedCause,
) error {

	if err := validationFn(); err != nil {
		if _, ok := err.(*serviceerror.InvalidArgument); ok {
			return handler.handlerFailDecision(failedCause, err.Error())
		}
		return err
	}

	return nil
}

func (handler *decisionTaskHandlerImpl) handlerFailDecision(
	failedCause eventpb.DecisionTaskFailedCause,
	failMessage string,
) error {
	handler.failDecisionInfo = &failDecisionInfo{
		cause:   failedCause,
		message: failMessage,
	}
	handler.stopProcessing = true
	return nil
}
