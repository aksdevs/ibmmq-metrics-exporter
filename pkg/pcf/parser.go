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
	MQIAMO_OPENS    = 733 // MQI opens
	MQIAMO_CLOSES   = 709 // MQI closes
	MQIAMO_PUTS     = 735 // MQI puts
	MQIAMO_GETS     = 722 // MQI gets
	MQIAMO_COMMITS  = 710 // MQI commits
	MQIAMO_BACKOUTS = 704 // MQI backouts

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
	QueueName    string `json:"queue_name"`
	CurrentDepth int32  `json:"current_depth"`
	HighDepth    int32  `json:"high_depth"`
	InputCount   int32  `json:"input_count"`
	OutputCount  int32  `json:"output_count"`
	EnqueueCount int32  `json:"enqueue_count"`
	DequeueCount int32  `json:"dequeue_count"`
	HasReaders   bool   `json:"has_readers"`
	HasWriters   bool   `json:"has_writers"`
}

// ChannelStatistics represents channel-specific statistics
type ChannelStatistics struct {
	ChannelName    string `json:"channel_name"`
	ConnectionName string `json:"connection_name"`
	Messages       int32  `json:"messages"`
	Bytes          int64  `json:"bytes"`
	Batches        int32  `json:"batches"`
}

// MQIStatistics represents MQI-specific statistics
type MQIStatistics struct {
	ApplicationName string `json:"application_name"`
	ApplicationTag  string `json:"application_tag"`
	ConnectionName  string `json:"connection_name"`
	UserIdentifier  string `json:"user_identifier"`
	ChannelName     string `json:"channel_name"`
	Opens           int32  `json:"opens"`
	Closes          int32  `json:"closes"`
	Puts            int32  `json:"puts"`
	Gets            int32  `json:"gets"`
	Commits         int32  `json:"commits"`
	Backouts        int32  `json:"backouts"`
}

// AccountingData represents parsed accounting data
type AccountingData struct {
	Type           string                 `json:"type"`
	QueueManager   string                 `json:"queue_manager"`
	Timestamp      time.Time              `json:"timestamp"`
	Parameters     map[string]interface{} `json:"parameters"`
	ConnectionInfo *ConnectionInfo        `json:"connection_info,omitempty"`
	Operations     *OperationCounts       `json:"operations,omitempty"`
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

	return acct, nil
}

// parseQueueStats extracts queue statistics from parameters
func (p *Parser) parseQueueStats(parameters []*PCFParameter) *QueueStatistics {
	stats := &QueueStatistics{}

	for _, param := range parameters {
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
			}
		}
	}

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
			// Sum all values in the list
			var sum int32
			for _, v := range valList {
				sum += int32(v)
			}

			switch param.Parameter {
			case MQIAMO_OPENS:
				stats.Opens = sum
				p.logger.WithField("opens", sum).Debug("Found Opens (from list)")
			case MQIAMO_CLOSES:
				stats.Closes = sum
				p.logger.WithField("closes", sum).Debug("Found Closes (from list)")
			case MQIAMO_PUTS:
				stats.Puts = sum
				p.logger.WithField("puts", sum).Debug("Found Puts (from list)")
			case MQIAMO_GETS:
				stats.Gets = sum
				p.logger.WithField("gets", sum).Debug("Found Gets (from list)")
			case MQIAMO_COMMITS:
				stats.Commits = sum
				p.logger.WithField("commits", sum).Debug("Found Commits (from list)")
			case MQIAMO_BACKOUTS:
				stats.Backouts = sum
				p.logger.WithField("backouts", sum).Debug("Found Backouts (from list)")
			}
			continue
		}

		if val, ok := param.Value.(int32); ok {
			switch param.Parameter {
			case MQIAMO_OPENS:
				stats.Opens = val
				p.logger.WithField("opens", val).Info("Found Opens")
			case MQIAMO_CLOSES:
				stats.Closes = val
				p.logger.WithField("closes", val).Info("Found Closes")
			case MQIAMO_PUTS:
				stats.Puts = val
				p.logger.WithField("puts", val).Info("Found Puts")
			case MQIAMO_GETS:
				stats.Gets = val
				p.logger.WithField("gets", val).Info("Found Gets")
			case MQIAMO_COMMITS:
				stats.Commits = val
				p.logger.WithField("commits", val).Info("Found Commits")
			case MQIAMO_BACKOUTS:
				stats.Backouts = val
				p.logger.WithField("backouts", val).Info("Found Backouts")
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
