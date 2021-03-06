package sqldb

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/auctioneer"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/metric"
	"github.com/pivotal-golang/lager"
)

const (
	convergeTaskRunsCounter = metric.Counter("ConvergenceTaskRuns")
	convergeTaskDuration    = metric.Duration("ConvergenceTaskDuration")

	tasksKickedCounter = metric.Counter("ConvergenceTasksKicked")
	tasksPrunedCounter = metric.Counter("ConvergenceTasksPruned")

	pendingTasks   = metric.Metric("TasksPending")
	runningTasks   = metric.Metric("TasksRunning")
	completedTasks = metric.Metric("TasksCompleted")
	resolvingTasks = metric.Metric("TasksResolving")
)

func (db *SQLDB) ConvergeTasks(logger lager.Logger, cellSet models.CellSet, kickTasksDuration, expirePendingTaskDuration, expireCompletedTaskDuration time.Duration) ([]*auctioneer.TaskStartRequest, []*models.Task) {
	logger.Info("starting")
	defer logger.Info("completed")

	convergeTaskRunsCounter.Increment()
	convergeStart := db.clock.Now()

	defer func() {
		err := convergeTaskDuration.Send(time.Since(convergeStart))
		if err != nil {
			logger.Error("failed-to-send-converge-task-duration-metric", err)
		}
	}()

	var tasksPruned, tasksKicked uint64

	rowsAffected := db.failExpiredPendingTasks(logger, expirePendingTaskDuration)
	tasksKicked += uint64(rowsAffected)

	tasksToAuction, failedFetches := db.getTaskStartRequestsForKickablePendingTasks(logger, kickTasksDuration, expirePendingTaskDuration)
	tasksPruned += failedFetches
	tasksKicked += uint64(len(tasksToAuction))

	rowsAffected = db.failTasksWithDisappearedCells(logger, cellSet)
	tasksKicked += uint64(rowsAffected)

	// do this first so that we now have "Completed" tasks before cleaning up
	// or re-sending the completion callback
	db.demoteKickableResolvingTasks(logger, kickTasksDuration)

	rowsAffected = db.deleteExpiredCompletedTasks(logger, expireCompletedTaskDuration)
	tasksPruned += uint64(rowsAffected)

	tasksToComplete, failedFetches := db.getKickableCompleteTasksForCompletion(logger, kickTasksDuration)
	tasksPruned += failedFetches
	tasksKicked += uint64(len(tasksToComplete))

	pendingCount, runningCount, completedCount, resolvingCount := db.countTasksByState(logger.Session("count-tasks"), db.db)

	sendTaskMetrics(logger, pendingCount, runningCount, completedCount, resolvingCount)

	tasksKickedCounter.Add(tasksKicked)
	tasksPrunedCounter.Add(tasksPruned)

	return tasksToAuction, tasksToComplete
}

func (db *SQLDB) failExpiredPendingTasks(logger lager.Logger, expirePendingTaskDuration time.Duration) int64 {
	logger = logger.Session("fail-expired-pending-tasks")

	now := db.clock.Now()

	result, err := db.update(logger, db.db, tasksTable,
		SQLAttributes{
			"failed":             true,
			"failure_reason":     "not started within time limit",
			"result":             "",
			"state":              models.Task_Completed,
			"first_completed_at": now.UnixNano(),
			"updated_at":         now.UnixNano(),
		},
		"state = ? AND created_at < ?", models.Task_Pending, now.Add(-expirePendingTaskDuration).UnixNano())
	if err != nil {
		logger.Error("failed-query", err)
		return 0
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("failed-rows-affected", err)
		return 0
	}
	return rowsAffected
}

func (db *SQLDB) getTaskStartRequestsForKickablePendingTasks(logger lager.Logger, kickTasksDuration, expirePendingTaskDuration time.Duration) ([]*auctioneer.TaskStartRequest, uint64) {
	logger = logger.Session("get-task-start-requests-for-kickable-pending-tasks")

	rows, err := db.all(logger, db.db, tasksTable,
		taskColumns, NoLockRow,
		"state = ? AND updated_at < ? AND created_at > ?",
		models.Task_Pending, db.clock.Now().Add(-kickTasksDuration).UnixNano(), db.clock.Now().Add(-expirePendingTaskDuration).UnixNano(),
	)
	if err != nil {
		logger.Error("failed-query", err)
	}
	defer rows.Close()

	var failedFetches uint64
	tasksToAuction := []*auctioneer.TaskStartRequest{}
	for rows.Next() {
		task, err := db.fetchTask(logger, rows, db.db)
		if err != nil {
			logger.Error("failed-fetch", err)
			if err == models.ErrDeserialize {
				failedFetches++
			}
		} else {
			taskStartRequest := auctioneer.NewTaskStartRequestFromModel(task.TaskGuid, task.Domain, task.TaskDefinition)
			tasksToAuction = append(tasksToAuction, &taskStartRequest)
		}
	}

	if rows.Err() != nil {
		logger.Error("failed-getting-next-row", rows.Err())
	}

	return tasksToAuction, failedFetches
}

func (db *SQLDB) failTasksWithDisappearedCells(logger lager.Logger, cellSet models.CellSet) int64 {
	logger = logger.Session("fail-tasks-with-disappeared-cells")

	values := make([]interface{}, 0, 1+len(cellSet))
	values = append(values, models.Task_Running)

	for k := range cellSet {
		values = append(values, k)
	}

	wheres := "state = ?"
	if len(cellSet) != 0 {
		wheres += fmt.Sprintf(" AND cell_id NOT IN (%s)", questionMarks(len(cellSet)))
	}
	now := db.clock.Now().UnixNano()

	result, err := db.update(logger, db.db, tasksTable,
		SQLAttributes{
			"failed":             true,
			"failure_reason":     "cell disappeared before completion",
			"result":             "",
			"state":              models.Task_Completed,
			"first_completed_at": now,
			"updated_at":         now,
		},
		wheres, values...,
	)
	if err != nil {
		logger.Error("failed-updating-tasks", err)
		return 0
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("failed-rows-affected", err)
		return 0
	}

	return rowsAffected
}

func (db *SQLDB) demoteKickableResolvingTasks(logger lager.Logger, kickTasksDuration time.Duration) {
	logger = logger.Session("demote-kickable-resolving-tasks")
	_, err := db.update(logger, db.db, tasksTable,
		SQLAttributes{"state": models.Task_Completed},
		"state = ? AND updated_at < ?",
		models.Task_Resolving, db.clock.Now().Add(-kickTasksDuration).UnixNano(),
	)
	if err != nil {
		logger.Error("failed-updating-tasks", err)
	}
}

func (db *SQLDB) deleteExpiredCompletedTasks(logger lager.Logger, expireCompletedTaskDuration time.Duration) int64 {
	logger = logger.Session("delete-expired-completed-tasks")

	result, err := db.delete(logger, db.db, tasksTable, "state = ? AND first_completed_at < ?", models.Task_Completed, db.clock.Now().Add(-expireCompletedTaskDuration).UnixNano())
	if err != nil {
		logger.Error("failed-query", err)
		return 0
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Error("failed-rows-affected", err)
		return 0
	}

	return rowsAffected
}

func (db *SQLDB) getKickableCompleteTasksForCompletion(logger lager.Logger, kickTasksDuration time.Duration) ([]*models.Task, uint64) {
	logger = logger.Session("get-kickable-complete-tasks-for-completion")

	rows, err := db.all(logger, db.db, tasksTable,
		taskColumns, NoLockRow,
		"state = ? AND updated_at < ?",
		models.Task_Completed, db.clock.Now().Add(-kickTasksDuration).UnixNano(),
	)
	if err != nil {
		logger.Error("failed-query", err)
	}
	defer rows.Close()

	var failedFetches uint64
	tasksToComplete := []*models.Task{}
	for rows.Next() {
		task, err := db.fetchTask(logger, rows, db.db)
		if err != nil {
			logger.Error("failed-fetch", err)
			if err == models.ErrDeserialize {
				failedFetches++
			}
		} else {
			tasksToComplete = append(tasksToComplete, task)
		}
	}

	if rows.Err() != nil {
		logger.Error("failed-getting-next-row", rows.Err())
	}

	return tasksToComplete, failedFetches
}

func sendTaskMetrics(logger lager.Logger, pendingCount, runningCount, completedCount, resolvingCount int) {
	err := pendingTasks.Send(pendingCount)
	if err != nil {
		logger.Error("failed-to-send-pending-tasks-metric", err)
	}

	err = runningTasks.Send(runningCount)
	if err != nil {
		logger.Error("failed-to-send-running-tasks-metric", err)
	}

	err = completedTasks.Send(completedCount)
	if err != nil {
		logger.Error("failed-to-send-completed-tasks-metric", err)
	}

	err = resolvingTasks.Send(resolvingCount)
	if err != nil {
		logger.Error("failed-to-send-resolving-tasks-metric", err)
	}
}
