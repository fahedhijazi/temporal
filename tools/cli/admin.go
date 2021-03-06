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

package cli

import "github.com/urfave/cli"

func newAdminWorkflowCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "show",
			Aliases: []string{"show"},
			Usage:   "show workflow history from database",
			Flags: []cli.Flag{
				// v2 history events
				cli.StringFlag{
					Name:  FlagTreeID,
					Usage: "TreeId",
				},
				cli.StringFlag{
					Name:  FlagBranchID,
					Usage: "BranchId",
				},
				cli.StringFlag{
					Name:  FlagOutputFilenameWithAlias,
					Usage: "output file",
				},

				// for persistence connection
				// TODO need to support other database: https://github.com/uber/cadence/issues/2777
				cli.StringFlag{
					Name:  FlagDBAddress,
					Usage: "persistence address(right now only cassandra is supported)",
				},
				cli.IntFlag{
					Name:  FlagDBPort,
					Value: 9042,
					Usage: "persistence port",
				},
				cli.StringFlag{
					Name:  FlagUsername,
					Usage: "cassandra username",
				},
				cli.StringFlag{
					Name:  FlagPassword,
					Usage: "cassandra password",
				},
				cli.StringFlag{
					Name:  FlagKeyspace,
					Usage: "cassandra keyspace",
				},
				cli.BoolFlag{
					Name:  FlagEnableTLS,
					Usage: "enable TLS over cassandra connection",
				},
				cli.StringFlag{
					Name:  FlagTLSCertPath,
					Usage: "cassandra tls client cert path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSKeyPath,
					Usage: "cassandra tls client key path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSCaPath,
					Usage: "cassandra tls client ca path (tls must be enabled)",
				},
				cli.BoolFlag{
					Name:  FlagTLSEnableHostVerification,
					Usage: "cassandra tls verify hostname and server cert (tls must be enabled)",
				},

				// support mysql query
				cli.IntFlag{
					Name:  FlagShardIDWithAlias,
					Usage: "ShardId",
				},
			},
			Action: func(c *cli.Context) {
				AdminShowWorkflow(c)
			},
		},
		{
			Name:    "describe",
			Aliases: []string{"desc"},
			Usage:   "Describe internal information of workflow execution",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.StringFlag{
					Name:  FlagRunIDWithAlias,
					Usage: "RunId",
				},
			},
			Action: func(c *cli.Context) {
				AdminDescribeWorkflow(c)
			},
		},
		{
			Name:    "refresh-tasks",
			Aliases: []string{"rt"},
			Usage:   "Refreshes all the tasks of a workflow",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.StringFlag{
					Name:  FlagRunIDWithAlias,
					Usage: "RunId",
				},
			},
			Action: func(c *cli.Context) {
				AdminRefreshWorkflowTasks(c)
			},
		},
		{
			Name:    "delete",
			Aliases: []string{"del"},
			Usage:   "Delete current workflow execution and the mutableState record",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.StringFlag{
					Name:  FlagRunIDWithAlias,
					Usage: "RunId",
				},
				cli.BoolFlag{
					Name:  FlagSkipErrorModeWithAlias,
					Usage: "skip errors when deleting history",
				},

				// for persistence connection
				// TODO need to support other database: https://github.com/uber/cadence/issues/2777
				cli.StringFlag{
					Name:  FlagDBAddress,
					Usage: "persistence address(right now only cassandra is supported)",
				},
				cli.IntFlag{
					Name:  FlagDBPort,
					Value: 9042,
					Usage: "persistence port",
				},
				cli.StringFlag{
					Name:  FlagUsername,
					Usage: "cassandra username",
				},
				cli.StringFlag{
					Name:  FlagPassword,
					Usage: "cassandra password",
				},
				cli.StringFlag{
					Name:  FlagKeyspace,
					Usage: "cassandra keyspace",
				},
				cli.BoolFlag{
					Name:  FlagEnableTLS,
					Usage: "use TLS over cassandra connection",
				},
				cli.StringFlag{
					Name:  FlagTLSCertPath,
					Usage: "cassandra tls client cert path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSKeyPath,
					Usage: "cassandra tls client key path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSCaPath,
					Usage: "cassandra tls client ca path (tls must be enabled)",
				},
				cli.BoolFlag{
					Name:  FlagTLSEnableHostVerification,
					Usage: "cassandra tls verify hostname and server cert (tls must be enabled)",
				},
			},
			Action: func(c *cli.Context) {
				AdminDeleteWorkflow(c)
			},
		},
	}
}

func newAdminShardManagementCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "closeShard",
			Aliases: []string{"clsh"},
			Usage:   "close a shard given a shard id",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  FlagShardID,
					Usage: "ShardId for the temporal cluster to manage",
				},
			},
			Action: func(c *cli.Context) {
				AdminShardManagement(c)
			},
		},
		{
			Name:    "removeTask",
			Aliases: []string{"rmtk"},
			Usage:   "remove a task based on shardId, typeId and taskId",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  FlagShardID,
					Usage: "ShardId for the temporal cluster to manage",
				},
				cli.Int64Flag{
					Name:  FlagRemoveTaskID,
					Usage: "task id which user want to specify",
				},
				cli.IntFlag{
					Name:  FlagRemoveTypeID,
					Usage: "type id which user want to specify: 2 (transfer task), 3 (timer task), 4 (replication task)",
				},
			},
			Action: func(c *cli.Context) {
				AdminRemoveTask(c)
			},
		},
	}
}

func newAdminHistoryHostCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "describe",
			Aliases: []string{"desc"},
			Usage:   "Describe internal information of history host",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.StringFlag{
					Name:  FlagHistoryAddressWithAlias,
					Usage: "History Host address(IP:PORT)",
				},
				cli.IntFlag{
					Name:  FlagShardIDWithAlias,
					Usage: "ShardId",
				},
				cli.BoolFlag{
					Name:  FlagPrintFullyDetailWithAlias,
					Usage: "Print fully detail",
				},
			},
			Action: func(c *cli.Context) {
				AdminDescribeHistoryHost(c)
			},
		},
		{
			Name:    "getshard",
			Aliases: []string{"gsh"},
			Usage:   "Get shardId for a workflowId",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.IntFlag{
					Name:  FlagNumberOfShards,
					Usage: "NumberOfShards for the temporal cluster(see config for numHistoryShards)",
				},
			},
			Action: func(c *cli.Context) {
				AdminGetShardID(c)
			},
		},
	}
}

func newAdminNamespaceCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "register",
			Aliases: []string{"re"},
			Usage:   "Register workflow namespace",
			Flags:   adminRegisterNamespaceFlags,
			Action: func(c *cli.Context) {
				newNamespaceCLI(c, true).RegisterNamespace(c)
			},
		},
		{
			Name:    "update",
			Aliases: []string{"up", "u"},
			Usage:   "Update existing workflow namespace",
			Flags:   adminUpdateNamespaceFlags,
			Action: func(c *cli.Context) {
				newNamespaceCLI(c, true).UpdateNamespace(c)
			},
		},
		{
			Name:    "describe",
			Aliases: []string{"desc"},
			Usage:   "Describe existing workflow namespace",
			Flags:   adminDescribeNamespaceFlags,
			Action: func(c *cli.Context) {
				newNamespaceCLI(c, true).DescribeNamespace(c)
			},
		},
		{
			Name:    "getnamespaceidorname",
			Aliases: []string{"getdn"},
			Usage:   "Get namespaceId or namespace",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagNamespace,
					Usage: "Namespace",
				},
				cli.StringFlag{
					Name:  FlagNamespaceID,
					Usage: "Namespace Id(uuid)",
				},

				// for persistence connection
				// TODO need to support other database: https://github.com/uber/cadence/issues/2777
				cli.StringFlag{
					Name:  FlagDBAddress,
					Usage: "persistence address(right now only cassandra is supported)",
				},
				cli.IntFlag{
					Name:  FlagDBPort,
					Value: 9042,
					Usage: "persistence port",
				},
				cli.StringFlag{
					Name:  FlagUsername,
					Usage: "cassandra username",
				},
				cli.StringFlag{
					Name:  FlagPassword,
					Usage: "cassandra password",
				},
				cli.StringFlag{
					Name:  FlagKeyspace,
					Usage: "cassandra keyspace",
				},
				cli.BoolFlag{
					Name:  FlagEnableTLS,
					Usage: "use TLS over cassandra connection",
				},
				cli.StringFlag{
					Name:  FlagTLSCertPath,
					Usage: "cassandra tls client cert path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSKeyPath,
					Usage: "cassandra tls client key path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSCaPath,
					Usage: "cassandra tls client ca path (tls must be enabled)",
				},
				cli.BoolFlag{
					Name:  FlagTLSEnableHostVerification,
					Usage: "cassandra tls verify hostname and server cert (tls must be enabled)",
				},
			},
			Action: func(c *cli.Context) {
				AdminGetNamespaceIDOrName(c)
			},
		},
	}
}

func newAdminKafkaCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "parse",
			Aliases: []string{"par"},
			Usage:   "Parse replication tasks from kafka messages",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagInputFileWithAlias,
					Usage: "Input file to use, if not present assumes piping",
				},
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId, if not provided then no filters by WorkflowId are applied",
				},
				cli.StringFlag{
					Name:  FlagRunIDWithAlias,
					Usage: "RunId, if not provided then no filters by RunId are applied",
				},
				cli.StringFlag{
					Name:  FlagOutputFilenameWithAlias,
					Usage: "Output file to write to, if not provided output is written to stdout",
				},
				cli.BoolFlag{
					Name:  FlagSkipErrorModeWithAlias,
					Usage: "Skip errors in parsing messages",
				},
				cli.BoolFlag{
					Name:  FlagHeadersModeWithAlias,
					Usage: "Output headers of messages in format: NamespaceId, WorkflowId, RunId, FirstEventId, NextEventId",
				},
				cli.IntFlag{
					Name:  FlagMessageTypeWithAlias,
					Usage: "Kafka message type (0: replicationTasks; 1: visibility)",
					Value: 0,
				},
			},
			Action: func(c *cli.Context) {
				AdminKafkaParse(c)
			},
		},
		{
			Name:    "purgeTopic",
			Aliases: []string{"purge"},
			Usage:   "purge Kafka topic by consumer group",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagCluster,
					Usage: "Name of the Kafka cluster to publish replicationTasks",
				},
				cli.StringFlag{
					Name:  FlagTopic,
					Usage: "Topic to publish replication task",
				},
				cli.StringFlag{
					Name:  FlagGroup,
					Usage: "Group to read DLQ",
				},
				cli.StringFlag{
					Name: FlagHostFile,
					Usage: "Kafka host config file in format of: " + `
tls:
    enabled: false
    certFile: ""
    keyFile: ""
    caFile: ""
clusters:
	localKafka:
		brokers:
		- 127.0.0.1
		- 127.0.0.2`,
				},
			},
			Action: func(c *cli.Context) {
				AdminPurgeTopic(c)
			},
		},
		{
			Name:    "mergeDLQ",
			Aliases: []string{"mgdlq"},
			Usage:   "Merge replication tasks to target topic(from input file or DLQ topic)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagInputFileWithAlias,
					Usage: "Input file to use to read as JSON of ReplicationTask, separated by line",
				},
				cli.StringFlag{
					Name:  FlagInputTopicWithAlias,
					Usage: "Input topic to read ReplicationTask",
				},
				cli.StringFlag{
					Name:  FlagInputCluster,
					Usage: "Name of the Kafka cluster for reading DLQ topic for ReplicationTask",
				},
				cli.Int64Flag{
					Name:  FlagStartOffset,
					Usage: "Starting offset for reading DLQ topic for ReplicationTask",
				},
				cli.StringFlag{
					Name:  FlagCluster,
					Usage: "Name of the Kafka cluster to publish replicationTasks",
				},
				cli.StringFlag{
					Name:  FlagTopic,
					Usage: "Topic to publish replication task",
				},
				cli.StringFlag{
					Name:  FlagGroup,
					Usage: "Group to read DLQ",
				},
				cli.StringFlag{
					Name: FlagHostFile,
					Usage: "Kafka host config file in format of: " + `
tls:
    enabled: false
    certFile: ""
    keyFile: ""
    caFile: ""
clusters:
	localKafka:
		brokers:
		- 127.0.0.1
		- 127.0.0.2`,
				},
			},
			Action: func(c *cli.Context) {
				AdminMergeDLQ(c)
			},
		},
		{
			Name:    "rereplicate",
			Aliases: []string{"rrp"},
			Usage:   "Rereplicate replication tasks to target topic from history tables",
			Flags: []cli.Flag{

				cli.StringFlag{
					Name:  FlagTargetCluster,
					Usage: "Name of targetCluster to receive the replication task",
				},
				cli.IntFlag{
					Name:  FlagNumberOfShards,
					Usage: "NumberOfShards is required to calculate shardId. (see server config for numHistoryShards)",
				},

				// for multiple workflow
				cli.StringFlag{
					Name:  FlagInputFileWithAlias,
					Usage: "Input file to read multiple workflow line by line. For each line: namespaceId workflowId,runId,minEventId,maxEventId (minEventId/maxEventId are optional.)",
				},

				// for one workflow
				cli.Int64Flag{
					Name:  FlagMinEventID,
					Usage: "MinEventId. Optional, default to all events",
				},
				cli.Int64Flag{
					Name:  FlagMaxEventID,
					Usage: "MaxEventId Optional, default to all events",
				},
				cli.StringFlag{
					Name:  FlagWorkflowIDWithAlias,
					Usage: "WorkflowId",
				},
				cli.StringFlag{
					Name:  FlagRunIDWithAlias,
					Usage: "RunId",
				},
				cli.StringFlag{
					Name:  FlagNamespaceID,
					Usage: "NamespaceId",
				},

				// for persistence connection
				// TODO need to support other database: https://github.com/uber/cadence/issues/2777
				cli.StringFlag{
					Name:  FlagDBAddress,
					Usage: "persistence address(right now only cassandra is supported)",
				},
				cli.IntFlag{
					Name:  FlagDBPort,
					Value: 9042,
					Usage: "persistence port",
				},
				cli.StringFlag{
					Name:  FlagUsername,
					Usage: "cassandra username",
				},
				cli.StringFlag{
					Name:  FlagPassword,
					Usage: "cassandra password",
				},
				cli.StringFlag{
					Name:  FlagKeyspace,
					Usage: "cassandra keyspace",
				},
				cli.BoolFlag{
					Name:  FlagEnableTLS,
					Usage: "use TLS over cassandra connection",
				},
				cli.StringFlag{
					Name:  FlagTLSCertPath,
					Usage: "cassandra tls client cert path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSKeyPath,
					Usage: "cassandra tls client key path (tls must be enabled)",
				},
				cli.StringFlag{
					Name:  FlagTLSCaPath,
					Usage: "cassandra tls client ca path (tls must be enabled)",
				},
				cli.BoolFlag{
					Name:  FlagTLSEnableHostVerification,
					Usage: "cassandra tls verify hostname and server cert (tls must be enabled)",
				},

				// kafka
				cli.StringFlag{
					Name:  FlagCluster,
					Usage: "Name of the Kafka cluster to publish replicationTasks",
				},
				cli.StringFlag{
					Name:  FlagTopic,
					Usage: "Topic to publish replication task",
				},
				cli.StringFlag{
					Name: FlagHostFile,
					Usage: "Kafka host config file in format of: " + `
tls:
    enabled: false
    certFile: ""
    keyFile: ""
    caFile: ""
clusters:
	localKafka:
		brokers:
		- 127.0.0.1
		- 127.0.0.2`,
				},
			},
			Action: func(c *cli.Context) {
				AdminRereplicate(c)
			},
		},
	}
}

func newAdminElasticSearchCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "catIndex",
			Aliases: []string{"cind"},
			Usage:   "Cat Indices on ElasticSearch",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagURL,
					Usage: "URL of ElasticSearch cluster",
				},
			},
			Action: func(c *cli.Context) {
				AdminCatIndices(c)
			},
		},
		{
			Name:    "index",
			Aliases: []string{"ind"},
			Usage:   "Index docs on ElasticSearch",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagURL,
					Usage: "URL of ElasticSearch cluster",
				},
				cli.StringFlag{
					Name:  FlagIndex,
					Usage: "ElasticSearch target index",
				},
				cli.StringFlag{
					Name:  FlagInputFileWithAlias,
					Usage: "Input file of indexergenpb.Message in json format, separated by newline",
				},
				cli.IntFlag{
					Name:  FlagBatchSizeWithAlias,
					Usage: "Optional batch size of actions for bulk operations",
					Value: 1000,
				},
			},
			Action: func(c *cli.Context) {
				AdminIndex(c)
			},
		},
		{
			Name:    "delete",
			Aliases: []string{"del"},
			Usage:   "Delete docs on ElasticSearch",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagURL,
					Usage: "URL of ElasticSearch cluster",
				},
				cli.StringFlag{
					Name:  FlagIndex,
					Usage: "ElasticSearch target index",
				},
				cli.StringFlag{
					Name: FlagInputFileWithAlias,
					Usage: "Input file name. Redirect temporal wf list result (with tale format) to a file and use as delete input. " +
						"First line should be table header like WORKFLOW TYPE | WORKFLOW ID | RUN ID | ...",
				},
				cli.IntFlag{
					Name:  FlagBatchSizeWithAlias,
					Usage: "Optional batch size of actions for bulk operations",
					Value: 1000,
				},
				cli.IntFlag{
					Name:  FlagRPS,
					Usage: "Optional batch request rate per second",
					Value: 30,
				},
			},
			Action: func(c *cli.Context) {
				AdminDelete(c)
			},
		},
		{
			Name:    "report",
			Aliases: []string{"rep"},
			Usage:   "Generate Report by Aggregation functions on ElasticSearch",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagURL,
					Usage: "URL of ElasticSearch cluster",
				},
				cli.StringFlag{
					Name:  FlagIndex,
					Usage: "ElasticSearch target index",
				},
				cli.StringFlag{
					Name:  FlagListQuery,
					Usage: "SQL query of the report",
				},
				cli.StringFlag{
					Name:  FlagOutputFormat,
					Usage: "Additional output format (html or csv)",
				},
				cli.StringFlag{
					Name:  FlagOutputFilename,
					Usage: "Additional output filename with path",
				},
			},
			Action: func(c *cli.Context) {
				GenerateReport(c)
			},
		},
	}
}

func newAdminTaskListCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "describe",
			Aliases: []string{"desc"},
			Usage:   "Describe pollers and status information of tasklist",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagTaskListWithAlias,
					Usage: "TaskList description",
				},
				cli.StringFlag{
					Name:  FlagTaskListTypeWithAlias,
					Value: "decision",
					Usage: "Optional TaskList type [decision|activity]",
				},
			},
			Action: func(c *cli.Context) {
				AdminDescribeTaskList(c)
			},
		},
	}
}

func newAdminClusterCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "add-search-attr",
			Aliases: []string{"asa"},
			Usage:   "whitelist search attribute",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagSearchAttributesKey,
					Usage: "Search Attribute key to be whitelisted",
				},
				cli.IntFlag{
					Name:  FlagSearchAttributesType,
					Value: -1,
					Usage: "Search Attribute value type. [0:String, 1:Keyword, 2:Int, 3:Double, 4:Bool, 5:Datetime]",
				},
				cli.StringFlag{
					Name:  FlagSecurityTokenWithAlias,
					Usage: "Optional token for security check",
				},
			},
			Action: func(c *cli.Context) {
				AdminAddSearchAttribute(c)
			},
		},
		{
			Name:    "describe",
			Aliases: []string{"d"},
			Usage:   "Describe cluster information",
			Action: func(c *cli.Context) {
				AdminDescribeCluster(c)
			},
		},
	}
}

func newAdminDLQCommands() []cli.Command {
	return []cli.Command{
		{
			Name:    "read",
			Aliases: []string{"r"},
			Usage:   "Read DLQ Messages",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagDLQTypeWithAlias,
					Usage: "Type of DLQ to manage. (Options: namespace, history)",
				},
				cli.IntFlag{
					Name:  FlagShardIDWithAlias,
					Usage: "ShardId",
				},
				cli.IntFlag{
					Name:  FlagMaxMessageCountWithAlias,
					Usage: "Max message size to fetch",
				},
				cli.IntFlag{
					Name:  FlagLastMessageID,
					Usage: "The upper boundary of the read message",
				},
				cli.StringFlag{
					Name:  FlagOutputFilenameWithAlias,
					Usage: "Output file to write to, if not provided output is written to stdout",
				},
			},
			Action: func(c *cli.Context) {
				AdminGetDLQMessages(c)
			},
		},
		{
			Name:    "purge",
			Aliases: []string{"p"},
			Usage:   "Delete DLQ messages with equal or smaller ids than the provided task id",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagDLQTypeWithAlias,
					Usage: "Type of DLQ to manage. (Options: namespace, history)",
				},
				cli.IntFlag{
					Name:  FlagShardIDWithAlias,
					Usage: "ShardId",
				},
				cli.IntFlag{
					Name:  FlagLastMessageID,
					Usage: "The upper boundary of the read message",
				},
			},
			Action: func(c *cli.Context) {
				AdminPurgeDLQMessages(c)
			},
		},
		{
			Name:    "merge",
			Aliases: []string{"m"},
			Usage:   "Merge DLQ messages with equal or smaller ids than the provided task id",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  FlagDLQTypeWithAlias,
					Usage: "Type of DLQ to manage. (Options: namespace, history)",
				},
				cli.IntFlag{
					Name:  FlagShardIDWithAlias,
					Usage: "ShardId",
				},
				cli.IntFlag{
					Name:  FlagLastMessageID,
					Usage: "The upper boundary of the read message",
				},
			},
			Action: func(c *cli.Context) {
				AdminMergeDLQMessages(c)
			},
		},
	}
}
