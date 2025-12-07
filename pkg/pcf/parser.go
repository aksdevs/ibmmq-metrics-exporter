package pcf

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/sirupsen/logrus"
)

// PCF Command Format Types (MQCFT_*)
const (
	MQCFT_NONE               = 0
	MQCFT_COMMAND            = 1
	MQCFT_RESPONSE           = 2
	MQCFT_INTEGER            = 3
	MQCFT_STRING             = 4
	MQCFT_INTEGER_LIST       = 5
	MQCFT_STRING_LIST        = 6
	MQCFT_EVENT              = 7
	MQCFT_USER               = 8
	MQCFT_BYTE_STRING        = 9
	MQCFT_TRACE_ROUTE        = 10
	MQCFT_REPORT             = 11
	MQCFT_INTEGER_FILTER     = 12
	MQCFT_STRING_FILTER      = 13
	MQCFT_BYTE_STRING_FILTER = 14
	MQCFT_COMMAND_XR         = 16
	MQCFT_XR_MSG             = 17
	MQCFT_XR_ITEM            = 18
	MQCFT_XR_SUMMARY         = 19
	MQCFT_GROUP              = 20
	MQCFT_STATISTICS         = 21
	MQCFT_ACCOUNTING         = 22
	MQCFT_INTEGER64          = 23
	MQCFT_INTEGER64_LIST     = 25
	MQCFT_APP_ACTIVITY       = 26
	// Type 68 seen in IBM MQ Windows statistics - possibly embedded or nested structure
	MQCFT_EMBEDDED_PCF = 68
)

// Common IBM MQ Constants
const (
	// Statistics Types (MQCMD_*)
	MQCMD_STATISTICS_MQI     = 112 // 0x70
	MQCMD_STATISTICS_Q       = 113 // 0x71
	MQCMD_STATISTICS_CHANNEL = 114 // 0x72
	MQCMD_Q_MGR_STATUS       = 164 // 0xA4 - Queue Manager Status (contains MQI stats)

	// Accounting Types (MQCMD_*)
	MQCMD_ACCOUNTING_MQI     = 138 // 0x8A
	MQCMD_ACCOUNTING_Q       = 139 // 0x8B
	MQCMD_ACCOUNTING_CHANNEL = 167 // 0xA7

	// Common Parameters (MQCA_*, MQIA_*)
	MQCA_Q_NAME            = 2016 // Queue name
	MQCA_Q_MGR_NAME        = 2002 // Queue manager name
	MQCA_CHANNEL_NAME      = 3501 // Channel name
	MQCA_CONNECTION_NAME   = 3502 // Connection name
	MQCA_APPL_NAME         = 2024 // Application name
	MQIA_Q_TYPE            = 20   // Queue type
	MQIA_CURRENT_Q_DEPTH   = 3    // Current queue depth
	MQIA_OPEN_INPUT_COUNT  = 65   // Open input count
	MQIA_OPEN_OUTPUT_COUNT = 66   // Open output count

	// Queue Statistics (MQIA_*)
	MQIA_HIGH_Q_DEPTH  = 36 // High queue depth
	MQIA_MSG_DEQ_COUNT = 38 // Messages dequeued (GET count)
	MQIA_MSG_ENQ_COUNT = 37 // Messages enqueued (PUT count)

	// Channel Statistics (MQIACH_*)
	MQIACH_MSGS    = 1501 // Channel messages
	MQIACH_BYTES   = 1502 // Channel bytes
	MQIACH_BATCHES = 1503 // Channel batches

	// MQI Statistics (MQIAMO_*) - Windows values from cmqc_windows.go
	// Note: These values differ from Linux! On Linux: OPENS=3, CLOSES=4, etc.
	// On Windows, they are in the 700+ range.
	MQIAMO_OPENS                = 733 // MQI opens
	MQIAMO_CLOSES               = 709 // MQI closes
	MQIAMO_PUTS                 = 735 // MQI puts
	MQIAMO_GETS                 = 722 // MQI gets
	MQIAMO_COMMITS              = 710 // MQI commits
	MQIAMO_BACKOUTS             = 704 // MQI backouts
	MQIAMO_BROWSES              = 725 // MQI browses
	MQIAMO_INQS                 = 727 // MQI inquires
	MQIAMO_SETS                 = 731 // MQI sets
	MQIAMO_DISC_CLOSE_TIMEOUT   = 765 // Disconnection close timeout
	MQIAMO_DISC_RESET_TIMEOUT   = 766 // Disconnection reset timeout
	MQIAMO_FAILS                = 767 // MQI failures
	MQIAMO_INCOMPLETE_BATCH     = 768 // Incomplete batch
	MQIAMO_INCOMPLETE_MSG       = 769 // Incomplete message
	MQIAMO_WAIT_INTERVAL        = 770 // Wait interval
	MQIAMO_SYNCPOINT_HEURISTIC  = 771 // Syncpoint heuristic
	MQIAMO_HEAPS                = 772 // Heaps
	MQIAMO_LOGICAL_CONNECTIONS  = 773 // Logical connections
	MQIAMO_PHYSICAL_CONNECTIONS = 774 // Physical connections
	MQIAMO_CURRENT_CONNS        = 775 // Current connections
	MQIAMO_PERSISTENT_MSGS      = 776 // Persistent messages
	MQIAMO_NON_PERSISTENT_MSGS  = 777 // Non-persistent messages
	MQIAMO_LONG_MSGS            = 778 // Long messages
	MQIAMO_SHORT_MSGS           = 779 // Short messages
	MQIAMO_QUEUE_TIME           = 781 // Queue time
	MQIAMO_QUEUE_TIME_MAX       = 783 // Queue time max
	MQIAMO_ELAPSED_TIME         = 784 // Elapsed time
	MQIAMO_ELAPSED_TIME_MAX     = 785 // Elapsed time max
	MQIAMO_CONN_TIME            = 786 // Connection time
	MQIAMO_CONN_TIME_MAX        = 787 // Connection time max
	MQIAMO_STAMP_ENABLED        = 788 // Stamp enabled

	// Statistics related parameters
	MQIACF_MSGS_RECEIVED      = 744 // Messages received
	MQIACF_MSGS_SENT          = 751 // Messages sent
	MQIACF_BYTES_RECEIVED     = 752 // Bytes received
	MQIACF_BYTES_SENT         = 753 // Bytes sent
	MQIACF_CHANNEL_STATUS     = 754 // Channel status
	MQIACF_CHANNEL_TYPE       = 755 // Channel type
	MQIACF_CHANNEL_ERRORS     = 757 // Channel errors
	MQIACF_CHANNEL_DISC_COUNT = 758 // Channel disconnect count
	MQIACF_CHANNEL_EXITNAME   = 760 // Channel exit name

	// Additional measurement parameters
	MQIAMO_BACKOUT_COUNT   = 745 // Backout count
	MQIAMO_COMMITS_COUNT   = 747 // Commits count
	MQIAMO_ROLLBACK_COUNT  = 748 // Rollback count
	MQIAMO_FULL_BATCHES    = 749 // Full batches
	MQIAMO_PARTIAL_BATCHES = 714 // Partial batches

	// Application and Connection Parameters (MQCACF_*, MQCACH_*)
	MQCACF_APPL_NAME       = 3024 // Application name
	MQCACF_APPL_TAG        = 3058 // Application tag
	MQCACF_USER_IDENTIFIER = 3025 // User identifier
	MQCACH_CONNECTION_NAME = 3506 // Client connection name/IP

	// Time and Control Parameters (MQCACF_*, MQIACF_*)
	MQCACF_COMMAND_TIME    = 3603 // Command time
	MQIACF_SEQUENCE_NUMBER = 1001 // Sequence number
)

// PCFHeader represents the PCF message header
type PCFHeader struct {
	Type           int32
	StrucLength    int32
	Version        int32
	Command        int32
	MsgSeqNumber   int32
	Control        int32
	CompCode       int32
	Reason         int32
	ParameterCount int32
}

// PCFParameter represents a PCF parameter
type PCFParameter struct {
	Parameter    int32
	Type         int32
	StrucLength  int32 // Total structure length including padding
	Value        interface{}
	StringLength int32 // Only used for string types
}

// StatisticsData represents parsed statistics data
type StatisticsData struct {
	Type         string                 `json:"type"`
	QueueManager string                 `json:"queue_manager"`
	Timestamp    time.Time              `json:"timestamp"`
	Parameters   map[string]interface{} `json:"parameters"`
	QueueStats   *QueueStatistics       `json:"queue_stats,omitempty"`
	ChannelStats *ChannelStatistics     `json:"channel_stats,omitempty"`
	MQIStats     *MQIStatistics         `json:"mqi_stats,omitempty"`
}

// QueueStatistics represents queue-specific statistics
type QueueStatistics struct {
	QueueName       string     `json:"queue_name"`
	CurrentDepth    int32      `json:"current_depth"`
	HighDepth       int32      `json:"high_depth"`
	InputCount      int32      `json:"input_count"`
	OutputCount     int32      `json:"output_count"`
	EnqueueCount    int32      `json:"enqueue_count"`
	DequeueCount    int32      `json:"dequeue_count"`
	HasReaders      bool       `json:"has_readers"`
	HasWriters      bool       `json:"has_writers"`
	AssociatedProcs []ProcInfo `json:"associated_procs,omitempty"`
}

// ProcInfo represents a process (input/output) associated with a queue
type ProcInfo struct {
	ApplicationName string `json:"application_name"`
	ProcessID       int32  `json:"process_id"` // Operating system process ID
	ConnectionName  string `json:"connection_name"`
	UserIdentifier  string `json:"user_identifier"`
	ChannelName     string `json:"channel_name"`
	Role            string `json:"role"` // "input" or "output" or "unknown"
}

// ChannelStatistics represents channel-specific statistics
type ChannelStatistics struct {
	ChannelName    string `json:"channel_name"`
	ConnectionName string `json:"connection_name"`
	Messages       int32  `json:"messages"`
	Bytes          int64  `json:"bytes"`
	Batches        int32  `json:"batches"`
}

// MQIStatistics represents MQI-specific statistics (all 59 parameters)
type MQIStatistics struct {
	ApplicationName     string `json:"application_name"`
	ApplicationTag      string `json:"application_tag"`
	ConnectionName      string `json:"connection_name"`
	UserIdentifier      string `json:"user_identifier"`
	ChannelName         string `json:"channel_name"`
	Opens               int32  `json:"opens"`
	Closes              int32  `json:"closes"`
	Puts                int32  `json:"puts"`
	Gets                int32  `json:"gets"`
	Commits             int32  `json:"commits"`
	Backouts            int32  `json:"backouts"`
	Browses             int32  `json:"browses"`
	Inqs                int32  `json:"inqs"`
	Sets                int32  `json:"sets"`
	DiscCloseTimeout    int32  `json:"disc_close_timeout"`
	DiscResetTimeout    int32  `json:"disc_reset_timeout"`
	Fails               int32  `json:"fails"`
	IncompleteBatch     int32  `json:"incomplete_batch"`
	IncompleteMsg       int32  `json:"incomplete_msg"`
	WaitInterval        int32  `json:"wait_interval"`
	SyncpointHeuristic  int32  `json:"syncpoint_heuristic"`
	Heaps               int32  `json:"heaps"`
	LogicalConnections  int32  `json:"logical_connections"`
	PhysicalConnections int32  `json:"physical_connections"`
	CurrentConns        int32  `json:"current_conns"`
	PersistentMsgs      int32  `json:"persistent_msgs"`
	NonPersistentMsgs   int32  `json:"non_persistent_msgs"`
	LongMsgs            int32  `json:"long_msgs"`
	ShortMsgs           int32  `json:"short_msgs"`
	QueueTime           int64  `json:"queue_time"`
	QueueTimeMax        int64  `json:"queue_time_max"`
	ElapsedTime         int64  `json:"elapsed_time"`
	ElapsedTimeMax      int64  `json:"elapsed_time_max"`
	ConnTime            int64  `json:"conn_time"`
	ConnTimeMax         int64  `json:"conn_time_max"`
	StampEnabled        int32  `json:"stamp_enabled"`
	MsgsReceived        int32  `json:"msgs_received"`
	MsgsSent            int32  `json:"msgs_sent"`
	BytesReceived       int64  `json:"bytes_received"`
	BytesSent           int64  `json:"bytes_sent"`
	ChannelStatus       int32  `json:"channel_status"`
	ChannelType         int32  `json:"channel_type"`
	ChannelErrors       int32  `json:"channel_errors"`
	ChannelDiscCount    int32  `json:"channel_disc_count"`
	ChannelExitName     int32  `json:"channel_exit_name"`
	BackoutCount        int64  `json:"backout_count"`
	CommitsCount        int64  `json:"commits_count"`
	RollbackCount       int64  `json:"rollback_count"`
	FullBatches         int32  `json:"full_batches"`
	PartialBatches      int32  `json:"partial_batches"`
}

// AccountingData represents parsed accounting data
type AccountingData struct {
	Type           string                 `json:"type"`
	QueueManager   string                 `json:"queue_manager"`
	Timestamp      time.Time              `json:"timestamp"`
	Parameters     map[string]interface{} `json:"parameters"`
	ConnectionInfo *ConnectionInfo        `json:"connection_info,omitempty"`
	Operations     *OperationCounts       `json:"operations,omitempty"`
	// Per-queue, per-application operation counts (from accounting groups)
	QueueOperations []*QueueAppOperation `json:"queue_operations,omitempty"`
}

// ConnectionInfo represents connection-specific accounting data
type ConnectionInfo struct {
	ChannelName     string    `json:"channel_name"`
	ConnectionName  string    `json:"connection_name"`
	ApplicationName string    `json:"application_name"`
	ApplicationTag  string    `json:"application_tag"`
	UserIdentifier  string    `json:"user_identifier"`
	ConnectTime     time.Time `json:"connect_time"`
	DisconnectTime  time.Time `json:"disconnect_time"`
}

// OperationCounts represents operation counts from accounting data
type OperationCounts struct {
	Gets     int32 `json:"gets"`
	Puts     int32 `json:"puts"`
	Browses  int32 `json:"browses"`
	Opens    int32 `json:"opens"`
	Closes   int32 `json:"closes"`
	Commits  int32 `json:"commits"`
	Backouts int32 `json:"backouts"`
}

// QueueAppOperation represents operation counts for a specific application/connection on a queue
type QueueAppOperation struct {
	QueueName       string `json:"queue_name"`
	ApplicationName string `json:"application_name"`
	ConnectionName  string `json:"connection_name"`
	UserIdentifier  string `json:"user_identifier"`
	Puts            int32  `json:"puts"`
	Gets            int32  `json:"gets"`
	MsgsReceived    int32  `json:"msgs_received"`
	MsgsSent        int32  `json:"msgs_sent"`
}

// Parser handles PCF message parsing
type Parser struct {
	logger *logrus.Logger
}

// NewParser creates a new PCF parser instance
func NewParser(logger *logrus.Logger) *Parser {
	return &Parser{
		logger: logger,
	}
}

// ParseMessage parses a PCF message and returns structured data
func (p *Parser) ParseMessage(data []byte, msgType string) (interface{}, error) {
	if len(data) < 36 { // Minimum PCF header size
		return nil, fmt.Errorf("message too short to be a valid PCF message")
	}

	header, err := p.parseHeader(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF header: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"command":         header.Command,
		"type":            header.Type,
		"parameter_count": header.ParameterCount,
		"message_type":    msgType,
	}).Debug("Parsing PCF message")

	parameters, err := p.parseParameters(data[36:], header.ParameterCount)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF parameters: %w", err)
	}

	// Determine if this is statistics or accounting data based on command
	switch {
	case header.Command == MQCMD_STATISTICS_Q:
		return p.parseStatistics(header, parameters)
	case header.Command == MQCMD_STATISTICS_CHANNEL:
		return p.parseStatistics(header, parameters)
	case header.Command == MQCMD_STATISTICS_MQI || header.Command == MQCMD_Q_MGR_STATUS:
		return p.parseStatistics(header, parameters)
	case header.Command == MQCMD_ACCOUNTING_Q || header.Command == MQCMD_ACCOUNTING_MQI || header.Command == MQCMD_ACCOUNTING_CHANNEL:
		return p.parseAccounting(header, parameters)
	default:
		// Generic parsing for other message types
		return &StatisticsData{
			Type:       msgType,
			Timestamp:  time.Now(),
			Parameters: p.convertParameters(parameters),
		}, nil
	}
}

// parseHeader parses the PCF header
func (p *Parser) parseHeader(data []byte) (*PCFHeader, error) {
	if len(data) < 36 {
		return nil, fmt.Errorf("insufficient data for PCF header")
	}

	header := &PCFHeader{
		Type:           int32(binary.LittleEndian.Uint32(data[0:4])),
		StrucLength:    int32(binary.LittleEndian.Uint32(data[4:8])),
		Version:        int32(binary.LittleEndian.Uint32(data[8:12])),
		Command:        int32(binary.LittleEndian.Uint32(data[12:16])),
		MsgSeqNumber:   int32(binary.LittleEndian.Uint32(data[16:20])),
		Control:        int32(binary.LittleEndian.Uint32(data[20:24])),
		CompCode:       int32(binary.LittleEndian.Uint32(data[24:28])),
		Reason:         int32(binary.LittleEndian.Uint32(data[28:32])),
		ParameterCount: int32(binary.LittleEndian.Uint32(data[32:36])),
	}

	return header, nil
}

// parseParameters parses PCF parameters using IBM's official library
func (p *Parser) parseParameters(data []byte, count int32) ([]*PCFParameter, error) {
	var parameters []*PCFParameter
	offset := 0

	for offset < len(data) {
		// Use IBM's ReadPCFParameter to parse each parameter
		ibmParam, bytesRead := ibmmq.ReadPCFParameter(data[offset:])
		if ibmParam == nil || bytesRead == 0 {
			p.logger.WithField("remaining_bytes", len(data)-offset).Debug("Failed to read PCF parameter")
			break
		}

		// Convert IBM's PCFParameter to our PCFParameter format
		param := p.convertIBMParameter(ibmParam)
		if param != nil {
			parameters = append(parameters, param)
		}

		offset += bytesRead
	}

	return parameters, nil
}

// convertIBMParameter converts IBM MQ library's PCFParameter to our format
func (p *Parser) convertIBMParameter(ibmParam *ibmmq.PCFParameter) *PCFParameter {
	if ibmParam == nil {
		return nil
	}

	param := &PCFParameter{
		Parameter: ibmParam.Parameter,
		Type:      ibmParam.Type,
	}

	// Convert value based on type
	switch ibmParam.Type {
	case MQCFT_INTEGER, MQCFT_INTEGER64:
		if len(ibmParam.Int64Value) > 0 {
			// For single integers, store as int32
			param.Value = int32(ibmParam.Int64Value[0])
		}
	case MQCFT_INTEGER_LIST, MQCFT_INTEGER64_LIST:
		// For integer lists, keep as []int64 but we'll need to handle this
		if len(ibmParam.Int64Value) > 0 {
			param.Value = ibmParam.Int64Value
		}
	case MQCFT_STRING:
		if len(ibmParam.String) > 0 {
			param.Value = p.cleanString(ibmParam.String[0])
		}
	case MQCFT_STRING_LIST:
		if len(ibmParam.String) > 0 {
			param.Value = ibmParam.String
		}
	case MQCFT_GROUP:
		// Recursively convert group parameters
		if len(ibmParam.GroupList) > 0 {
			nestedParams := make([]*PCFParameter, 0, len(ibmParam.GroupList))
			for _, ibmNested := range ibmParam.GroupList {
				if converted := p.convertIBMParameter(ibmNested); converted != nil {
					nestedParams = append(nestedParams, converted)
				}
			}
			if len(nestedParams) > 0 {
				param.Value = nestedParams
				p.logger.WithFields(logrus.Fields{
					"parameter":    param.Parameter,
					"nested_count": len(nestedParams),
				}).Debug("Converted GROUP parameter with nested params")
			}
		}
	case MQCFT_BYTE_STRING:
		if len(ibmParam.String) > 0 {
			// IBM library converts byte strings to hex strings
			param.Value = ibmParam.String[0]
		}
	default:
		p.logger.WithFields(logrus.Fields{
			"parameter": ibmParam.Parameter,
			"type":      ibmParam.Type,
		}).Debug("Unknown IBM parameter type")
	}

	return param
}

// parseStatistics converts parameters to statistics data structure
func (p *Parser) parseStatistics(header *PCFHeader, parameters []*PCFParameter) (*StatisticsData, error) {
	stats := &StatisticsData{
		Type:       "statistics",
		Timestamp:  time.Now(),
		Parameters: p.convertParameters(parameters),
	}

	// Extract common fields
	for _, param := range parameters {
		switch param.Parameter {
		case MQCA_Q_MGR_NAME:
			if str, ok := param.Value.(string); ok {
				stats.QueueManager = str
			}
		case MQCACF_COMMAND_TIME:
			// Parse MQ timestamp format if available
			if str, ok := param.Value.(string); ok {
				if t, err := p.parseMQTimestamp(str); err == nil {
					stats.Timestamp = t
				}
			}
		}
	}

	// Parse specific statistics based on command type
	switch header.Command {
	case MQCMD_STATISTICS_Q:
		stats.QueueStats = p.parseQueueStats(parameters)
	case MQCMD_STATISTICS_CHANNEL:
		stats.ChannelStats = p.parseChannelStats(parameters)
	case MQCMD_STATISTICS_MQI, MQCMD_Q_MGR_STATUS:
		stats.MQIStats = p.parseMQIStats(parameters)
	}

	return stats, nil
}

// parseAccounting converts parameters to accounting data structure
func (p *Parser) parseAccounting(header *PCFHeader, parameters []*PCFParameter) (*AccountingData, error) {
	acct := &AccountingData{
		Type:       "accounting",
		Timestamp:  time.Now(),
		Parameters: p.convertParameters(parameters),
	}

	// Extract common fields
	for _, param := range parameters {
		switch param.Parameter {
		case MQCA_Q_MGR_NAME:
			if str, ok := param.Value.(string); ok {
				acct.QueueManager = str
			}
		case MQCACF_COMMAND_TIME:
			if str, ok := param.Value.(string); ok {
				if t, err := p.parseMQTimestamp(str); err == nil {
					acct.Timestamp = t
				}
			}
		}
	}

	// Parse accounting-specific data
	acct.ConnectionInfo = p.parseConnectionInfo(parameters)
	acct.Operations = p.parseOperationCounts(parameters)

	// Extract per-queue per-application operations from GROUP parameters OR top-level arrays
	// Note: This is for queue-level accounting. MQI-level accounting (no queue names) is handled above.
	acct.QueueOperations = []*QueueAppOperation{}
	p.logger.WithFields(logrus.Fields{
		"total_params": len(parameters),
	}).Debug("parseAccounting: starting QueueOperations extraction")

	// First try GROUP-based extraction (for per-queue grouped data with queue names)
	groupFound := false
	for _, param := range parameters {
		p.logger.WithFields(logrus.Fields{
			"param_id":   param.Parameter,
			"param_type": param.Type,
			"value_type": fmt.Sprintf("%T", param.Value),
		}).Debug("parseAccounting: inspecting top-level parameter")

		if nested, ok := param.Value.([]*PCFParameter); ok {
			groupFound = true
			p.logger.WithFields(logrus.Fields{
				"param_id":     param.Parameter,
				"nested_count": len(nested),
			}).Info("parseAccounting: found GROUP with nested params")

			// A nested GROUP describes queue-level accounting info
			qa := &QueueAppOperation{}
			for _, np := range nested {
				p.logger.WithFields(logrus.Fields{
					"nested_param_id": np.Parameter,
					"nested_type":     np.Type,
					"value_type":      fmt.Sprintf("%T", np.Value),
					"value":           fmt.Sprintf("%v", np.Value),
				}).Debug("parseAccounting: nested parameter details")

				if valStr, ok := np.Value.(string); ok {
					switch np.Parameter {
					case MQCA_Q_NAME:
						qa.QueueName = valStr
						p.logger.WithField("queue_name", valStr).Info("parseAccounting: extracted QUEUE_NAME from GROUP")
					case MQCA_APPL_NAME, MQCACF_APPL_NAME:
						qa.ApplicationName = valStr
						p.logger.WithField("app_name", valStr).Info("parseAccounting: extracted APPL_NAME from GROUP")
					case MQCA_CONNECTION_NAME, MQCACH_CONNECTION_NAME:
						qa.ConnectionName = valStr
						p.logger.WithField("connection", valStr).Info("parseAccounting: extracted CONNECTION_NAME from GROUP")
					case MQCACF_USER_IDENTIFIER:
						qa.UserIdentifier = valStr
						p.logger.WithField("user", valStr).Info("parseAccounting: extracted USER_IDENTIFIER from GROUP")
					}
				}
				if valInt, ok := np.Value.(int32); ok {
					switch np.Parameter {
					case MQIAMO_PUTS:
						qa.Puts = valInt
						p.logger.WithField("puts", valInt).Info("parseAccounting: extracted PUTS from GROUP")
					case MQIAMO_GETS:
						qa.Gets = valInt
						p.logger.WithField("gets", valInt).Info("parseAccounting: extracted GETS from GROUP")
					case MQIACF_MSGS_RECEIVED:
						qa.MsgsReceived = valInt
						p.logger.WithField("msgs_received", valInt).Info("parseAccounting: extracted MSGS_RECEIVED from GROUP")
					case MQIACF_MSGS_SENT:
						qa.MsgsSent = valInt
						p.logger.WithField("msgs_sent", valInt).Info("parseAccounting: extracted MSGS_SENT from GROUP")
					}
				}
			}

			// Only append when we have a queue name and at least one operation
			if qa.QueueName != "" && (qa.Puts != 0 || qa.Gets != 0 || qa.MsgsReceived != 0 || qa.MsgsSent != 0) {
				acct.QueueOperations = append(acct.QueueOperations, qa)
				p.logger.WithFields(logrus.Fields{
					"queue_name":    qa.QueueName,
					"app_name":      qa.ApplicationName,
					"puts":          qa.Puts,
					"gets":          qa.Gets,
					"msgs_received": qa.MsgsReceived,
					"msgs_sent":     qa.MsgsSent,
				}).Info("parseAccounting: appended QueueAppOperation from GROUP")
			}
		}
	}

	// If no GROUP data found with queue names, this is MQI-level accounting (not queue-level)
	// MQI-level accounting is already handled via acct.Operations and acct.ConnectionInfo
	if !groupFound {
		p.logger.Info("parseAccounting: No GROUP with queue names found - this appears to be MQI-level accounting, not queue-level")
	}

	p.logger.WithField("total_queue_ops", len(acct.QueueOperations)).Info("parseAccounting: completed extraction")

	return acct, nil
}

// parseQueueStats extracts queue statistics from parameters
func (p *Parser) parseQueueStats(parameters []*PCFParameter) *QueueStatistics {
	stats := &QueueStatistics{}
	p.logger.WithField("total_params", len(parameters)).Debug("parseQueueStats: starting extraction")

	for _, param := range parameters {
		p.logger.WithFields(logrus.Fields{
			"param_id":   param.Parameter,
			"param_type": param.Type,
			"value_type": fmt.Sprintf("%T", param.Value),
		}).Debug("parseQueueStats: inspecting parameter")

		// Handle nested GROUP parameters that may represent individual processes (ipprocs/opprocs)
		if nested, ok := param.Value.([]*PCFParameter); ok {
			p.logger.WithFields(logrus.Fields{
				"param_id":     param.Parameter,
				"nested_count": len(nested),
			}).Info("parseQueueStats: found GROUP with nested params")

			// Extract connection/app info from nested group
			proc := ProcInfo{Role: "unknown"}
			for _, np := range nested {
				p.logger.WithFields(logrus.Fields{
					"nested_param_id": np.Parameter,
					"nested_type":     np.Type,
					"value":           fmt.Sprintf("%v", np.Value),
				}).Debug("parseQueueStats: nested param details")

				if val, ok := np.Value.(string); ok {
					switch np.Parameter {
					case MQCA_APPL_NAME, MQCACF_APPL_NAME:
						proc.ApplicationName = val
						p.logger.WithField("app_name", val).Info("parseQueueStats: extracted APPL_NAME")
					case MQCA_CONNECTION_NAME, MQCACH_CONNECTION_NAME:
						proc.ConnectionName = val
						p.logger.WithField("connection", val).Info("parseQueueStats: extracted CONNECTION_NAME")
					case MQCACF_USER_IDENTIFIER:
						proc.UserIdentifier = val
						p.logger.WithField("user", val).Info("parseQueueStats: extracted USER_IDENTIFIER")
					case MQCA_CHANNEL_NAME:
						proc.ChannelName = val
						p.logger.WithField("channel", val).Info("parseQueueStats: extracted CHANNEL_NAME")
					}
				}

				if ival, ok := np.Value.(int32); ok {
					switch np.Parameter {
					case MQIA_OPEN_INPUT_COUNT:
						if ival > 0 {
							proc.Role = "input"
							p.logger.WithField("input_count", ival).Info("parseQueueStats: detected input role")
						}
					case MQIA_OPEN_OUTPUT_COUNT:
						if ival > 0 {
							proc.Role = "output"
							p.logger.WithField("output_count", ival).Info("parseQueueStats: detected output role")
						}
					}
				}
			}

			// If we found application or connection info, append to associated procs
			if proc.ApplicationName != "" || proc.ConnectionName != "" || proc.UserIdentifier != "" {
				stats.AssociatedProcs = append(stats.AssociatedProcs, proc)
				p.logger.WithFields(logrus.Fields{
					"app_name":   proc.ApplicationName,
					"connection": proc.ConnectionName,
					"role":       proc.Role,
				}).Info("parseQueueStats: appended ProcInfo")
			}
			// continue processing other top-level params
			continue
		}

		if val, ok := param.Value.(int32); ok {
			switch param.Parameter {
			case MQIA_CURRENT_Q_DEPTH:
				stats.CurrentDepth = val
			case MQIA_HIGH_Q_DEPTH:
				stats.HighDepth = val
			case MQIA_OPEN_INPUT_COUNT:
				stats.InputCount = val
				stats.HasReaders = val > 0
			case MQIA_OPEN_OUTPUT_COUNT:
				stats.OutputCount = val
				stats.HasWriters = val > 0
			case MQIA_MSG_ENQ_COUNT:
				stats.EnqueueCount = val
			case MQIA_MSG_DEQ_COUNT:
				stats.DequeueCount = val
			}
		} else if str, ok := param.Value.(string); ok {
			switch param.Parameter {
			case MQCA_Q_NAME:
				stats.QueueName = str
				p.logger.WithFields(logrus.Fields{
					"queue_name":       str,
					"associated_procs": len(stats.AssociatedProcs),
				}).Info("parseQueueStats: extracted QUEUE_NAME")
			}
		}
	}

	p.logger.WithFields(logrus.Fields{
		"queue_name":       stats.QueueName,
		"associated_procs": len(stats.AssociatedProcs),
		"current_depth":    stats.CurrentDepth,
	}).Info("parseQueueStats: completed extraction")

	return stats
}

// parseChannelStats extracts channel statistics from parameters
func (p *Parser) parseChannelStats(parameters []*PCFParameter) *ChannelStatistics {
	stats := &ChannelStatistics{}

	for _, param := range parameters {
		if val, ok := param.Value.(int32); ok {
			switch param.Parameter {
			case MQIACH_MSGS:
				stats.Messages = val
			case MQIACH_BYTES:
				stats.Bytes = int64(val)
			case MQIACH_BATCHES:
				stats.Batches = val
			}
		} else if str, ok := param.Value.(string); ok {
			switch param.Parameter {
			case MQCA_CHANNEL_NAME:
				stats.ChannelName = str
			case MQCA_CONNECTION_NAME:
				stats.ConnectionName = str
			}
		}
	}

	return stats
}

// parseMQIStats extracts MQI statistics from parameters
func (p *Parser) parseMQIStats(parameters []*PCFParameter) *MQIStatistics {
	stats := &MQIStatistics{}

	// Log all parameters to help debug
	p.logger.WithField("parameter_count", len(parameters)).Debug("parseMQIStats called with parameters")

	for _, param := range parameters {
		p.logger.WithFields(logrus.Fields{
			"parameter_id": param.Parameter,
			"type":         param.Type,
			"value_type":   fmt.Sprintf("%T", param.Value),
		}).Debug("Processing parameter in MQI stats")

		// Check for nested parameters (from embedded PCF structures)
		if nestedParams, ok := param.Value.([]*PCFParameter); ok {
			p.logger.WithFields(logrus.Fields{
				"parameter_id": param.Parameter,
				"nested_count": len(nestedParams),
			}).Debug("Processing nested parameters in MQI stats")

			// Log first few nested param IDs to see what we have
			if len(nestedParams) > 0 {
				sampleIDs := []int32{}
				for i, np := range nestedParams {
					if i < 10 {
						sampleIDs = append(sampleIDs, np.Parameter)
					}
				}
				p.logger.WithField("sample_param_ids", sampleIDs).Debug("Sample nested parameter IDs")
			}

			// Recursively parse nested parameters
			nestedStats := p.parseMQIStats(nestedParams)
			if nestedStats != nil {
				// Merge nested stats into current stats
				if nestedStats.Opens > 0 {
					stats.Opens += nestedStats.Opens
				}
				if nestedStats.Closes > 0 {
					stats.Closes += nestedStats.Closes
				}
				if nestedStats.Puts > 0 {
					stats.Puts += nestedStats.Puts
				}
				if nestedStats.Gets > 0 {
					stats.Gets += nestedStats.Gets
				}
				if nestedStats.Commits > 0 {
					stats.Commits += nestedStats.Commits
				}
				if nestedStats.Backouts > 0 {
					stats.Backouts += nestedStats.Backouts
				}
				if nestedStats.ApplicationName != "" {
					stats.ApplicationName = nestedStats.ApplicationName
				}
			}
			continue
		}

		// Check for integer list values (MQCFT_INTEGER_LIST or MQCFT_INTEGER64_LIST)
		if valList, ok := param.Value.([]int64); ok {
			// Sum all values in the list or use first value for arrays
			var sum int32
			var sum64 int64
			for _, v := range valList {
				sum += int32(v)
				sum64 += v
			}

			switch param.Parameter {
			case MQIAMO_OPENS:
				stats.Opens = sum
			case MQIAMO_CLOSES:
				stats.Closes = sum
			case MQIAMO_PUTS:
				stats.Puts = sum
			case MQIAMO_GETS:
				stats.Gets = sum
			case MQIAMO_COMMITS:
				stats.Commits = sum
			case MQIAMO_BACKOUTS:
				stats.Backouts = sum
			case MQIAMO_BROWSES:
				stats.Browses = sum
			case MQIAMO_INQS:
				stats.Inqs = sum
			case MQIAMO_SETS:
				stats.Sets = sum
			case MQIACF_MSGS_RECEIVED:
				stats.MsgsReceived = sum
			case MQIACF_MSGS_SENT:
				stats.MsgsSent = sum
			case MQIACF_BYTES_RECEIVED:
				stats.BytesReceived = sum64
			case MQIACF_BYTES_SENT:
				stats.BytesSent = sum64
			case MQIAMO_PARTIAL_BATCHES:
				stats.PartialBatches = sum
			case MQIAMO_BACKOUT_COUNT:
				stats.BackoutCount = sum64
			case MQIAMO_COMMITS_COUNT:
				stats.CommitsCount = sum64
			case MQIAMO_ROLLBACK_COUNT:
				stats.RollbackCount = sum64
			case MQIAMO_SYNCPOINT_HEURISTIC:
				stats.SyncpointHeuristic = sum
			case MQIAMO_WAIT_INTERVAL:
				stats.WaitInterval = sum
			case MQIAMO_HEAPS:
				stats.Heaps = sum
			case MQIAMO_LOGICAL_CONNECTIONS:
				stats.LogicalConnections = sum
			case MQIAMO_PHYSICAL_CONNECTIONS:
				stats.PhysicalConnections = sum
			case MQIAMO_CURRENT_CONNS:
				stats.CurrentConns = sum
			case MQIAMO_PERSISTENT_MSGS:
				stats.PersistentMsgs = sum
			case MQIAMO_NON_PERSISTENT_MSGS:
				stats.NonPersistentMsgs = sum
			case MQIAMO_LONG_MSGS:
				stats.LongMsgs = sum
			case MQIAMO_SHORT_MSGS:
				stats.ShortMsgs = sum
			case MQIAMO_QUEUE_TIME:
				stats.QueueTime = sum64
			case MQIAMO_QUEUE_TIME_MAX:
				stats.QueueTimeMax = sum64
			case MQIAMO_ELAPSED_TIME:
				stats.ElapsedTime = sum64
			case MQIAMO_ELAPSED_TIME_MAX:
				stats.ElapsedTimeMax = sum64
			case MQIAMO_CONN_TIME:
				stats.ConnTime = sum64
			case MQIAMO_CONN_TIME_MAX:
				stats.ConnTimeMax = sum64
			}
			continue
		}

		if val, ok := param.Value.(int32); ok {
			switch param.Parameter {
			case MQIAMO_OPENS:
				stats.Opens = val
			case MQIAMO_CLOSES:
				stats.Closes = val
			case MQIAMO_PUTS:
				stats.Puts = val
			case MQIAMO_GETS:
				stats.Gets = val
			case MQIAMO_COMMITS:
				stats.Commits = val
			case MQIAMO_BACKOUTS:
				stats.Backouts = val
			case MQIAMO_BROWSES:
				stats.Browses = val
			case MQIAMO_INQS:
				stats.Inqs = val
			case MQIAMO_SETS:
				stats.Sets = val
			case MQIAMO_DISC_CLOSE_TIMEOUT:
				stats.DiscCloseTimeout = val
			case MQIAMO_DISC_RESET_TIMEOUT:
				stats.DiscResetTimeout = val
			case MQIAMO_FAILS:
				stats.Fails = val
			case MQIAMO_INCOMPLETE_BATCH:
				stats.IncompleteBatch = val
			case MQIAMO_INCOMPLETE_MSG:
				stats.IncompleteMsg = val
			case MQIAMO_WAIT_INTERVAL:
				stats.WaitInterval = val
			case MQIAMO_SYNCPOINT_HEURISTIC:
				stats.SyncpointHeuristic = val
			case MQIAMO_HEAPS:
				stats.Heaps = val
			case MQIAMO_LOGICAL_CONNECTIONS:
				stats.LogicalConnections = val
			case MQIAMO_PHYSICAL_CONNECTIONS:
				stats.PhysicalConnections = val
			case MQIAMO_CURRENT_CONNS:
				stats.CurrentConns = val
			case MQIAMO_PERSISTENT_MSGS:
				stats.PersistentMsgs = val
			case MQIAMO_NON_PERSISTENT_MSGS:
				stats.NonPersistentMsgs = val
			case MQIAMO_LONG_MSGS:
				stats.LongMsgs = val
			case MQIAMO_SHORT_MSGS:
				stats.ShortMsgs = val
			case MQIAMO_STAMP_ENABLED:
				stats.StampEnabled = val
			case MQIACF_MSGS_RECEIVED:
				stats.MsgsReceived = val
			case MQIACF_MSGS_SENT:
				stats.MsgsSent = val
			case MQIACF_CHANNEL_STATUS:
				stats.ChannelStatus = val
			case MQIACF_CHANNEL_TYPE:
				stats.ChannelType = val
			case MQIACF_CHANNEL_ERRORS:
				stats.ChannelErrors = val
			case MQIACF_CHANNEL_DISC_COUNT:
				stats.ChannelDiscCount = val
			case MQIACF_CHANNEL_EXITNAME:
				stats.ChannelExitName = val
			case MQIAMO_FULL_BATCHES:
				stats.FullBatches = val
			case MQIAMO_PARTIAL_BATCHES:
				stats.PartialBatches = val
			}
		} else if str, ok := param.Value.(string); ok {
			switch param.Parameter {
			case MQCA_APPL_NAME, MQCACF_APPL_NAME:
				stats.ApplicationName = str
				p.logger.WithField("app_name", str).Debug("Found ApplicationName")
			case MQCACF_APPL_TAG:
				stats.ApplicationTag = str
				p.logger.WithField("app_tag", str).Debug("Found ApplicationTag")
			case MQCACF_USER_IDENTIFIER:
				stats.UserIdentifier = str
				p.logger.WithField("user", str).Debug("Found UserIdentifier")
			case MQCACH_CONNECTION_NAME:
				stats.ConnectionName = str
				p.logger.WithField("connection", str).Debug("Found ConnectionName")
			case MQCA_CHANNEL_NAME:
				stats.ChannelName = str
				p.logger.WithField("channel", str).Debug("Found ChannelName")
			}
		}
	}

	return stats
}

// parseConnectionInfo extracts connection information from parameters
func (p *Parser) parseConnectionInfo(parameters []*PCFParameter) *ConnectionInfo {
	info := &ConnectionInfo{}

	for _, param := range parameters {
		if str, ok := param.Value.(string); ok {
			switch param.Parameter {
			case MQCA_CHANNEL_NAME:
				info.ChannelName = str
			case MQCA_CONNECTION_NAME, MQCACH_CONNECTION_NAME:
				info.ConnectionName = str
			case MQCA_APPL_NAME, MQCACF_APPL_NAME:
				info.ApplicationName = str
			case MQCACF_APPL_TAG:
				info.ApplicationTag = str
			case MQCACF_USER_IDENTIFIER:
				info.UserIdentifier = str
			}
		}
	}

	return info
}

// parseOperationCounts extracts operation counts from parameters
func (p *Parser) parseOperationCounts(parameters []*PCFParameter) *OperationCounts {
	ops := &OperationCounts{}

	for _, param := range parameters {
		if val, ok := param.Value.(int32); ok {
			switch param.Parameter {
			case MQIAMO_GETS:
				ops.Gets = val
			case MQIAMO_PUTS:
				ops.Puts = val
			case MQIAMO_OPENS:
				ops.Opens = val
			case MQIAMO_CLOSES:
				ops.Closes = val
			case MQIAMO_COMMITS:
				ops.Commits = val
			case MQIAMO_BACKOUTS:
				ops.Backouts = val
			}
		}
	}

	return ops
}

// convertParameters converts PCF parameters to a map for JSON serialization
func (p *Parser) convertParameters(parameters []*PCFParameter) map[string]interface{} {
	result := make(map[string]interface{})

	for _, param := range parameters {
		key := fmt.Sprintf("param_%d", param.Parameter)
		result[key] = param.Value
	}

	return result
}

// cleanString removes null terminators and trims whitespace
func (p *Parser) cleanString(s string) string {
	// Remove null terminators
	for i, r := range s {
		if r == 0 {
			s = s[:i]
			break
		}
	}
	// Trim whitespace
	return s
}

// parseMQTimestamp parses IBM MQ timestamp format
func (p *Parser) parseMQTimestamp(timestamp string) (time.Time, error) {
	// MQ timestamp format: YYYY-MM-DD HH:MM:SS.mmm
	// Try multiple formats
	formats := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"20060102150405",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timestamp)
}
