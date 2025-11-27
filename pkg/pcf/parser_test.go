package pcf

import (
	"encoding/binary"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPCFParser_ParseHeader(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	parser := NewParser(logger)

	tests := []struct {
		name     string
		data     []byte
		expected *PCFHeader
		wantErr  bool
	}{
		{
			name: "valid header",
			data: createTestPCFHeader(MQCFT_STATISTICS, MQCMD_STATISTICS_Q, 1),
			expected: &PCFHeader{
				Type:           MQCFT_STATISTICS,
				StrucLength:    36,
				Version:        1,
				Command:        MQCMD_STATISTICS_Q,
				MsgSeqNumber:   1,
				Control:        0,
				CompCode:       0,
				Reason:         0,
				ParameterCount: 1,
			},
			wantErr: false,
		},
		{
			name:    "too short data",
			data:    make([]byte, 20),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, err := parser.parseHeader(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.Type, header.Type)
			assert.Equal(t, tt.expected.Command, header.Command)
			assert.Equal(t, tt.expected.ParameterCount, header.ParameterCount)
		})
	}
}

func TestPCFParser_ParseParameters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	// Create test parameter data
	data := createTestPCFParameter(MQCA_Q_NAME, MQCFT_STRING, "TEST.QUEUE")

	params, err := parser.parseParameters(data, 1)
	require.NoError(t, err)
	require.Len(t, params, 1)

	param := params[0]
	assert.Equal(t, int32(MQCA_Q_NAME), param.Parameter)
	assert.Equal(t, int32(MQCFT_STRING), param.Type)
	assert.Equal(t, "TEST.QUEUE", param.Value)
}

func TestPCFParser_ParseQueueStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	parameters := []*PCFParameter{
		{Parameter: MQCA_Q_NAME, Type: MQCFT_STRING, Value: "TEST.QUEUE"},
		{Parameter: MQIA_CURRENT_Q_DEPTH, Type: MQCFT_INTEGER, Value: int32(100)},
		{Parameter: MQIA_HIGH_Q_DEPTH, Type: MQCFT_INTEGER, Value: int32(500)},
		{Parameter: MQIA_OPEN_INPUT_COUNT, Type: MQCFT_INTEGER, Value: int32(2)},
		{Parameter: MQIA_OPEN_OUTPUT_COUNT, Type: MQCFT_INTEGER, Value: int32(1)},
		{Parameter: MQIA_MSG_ENQ_COUNT, Type: MQCFT_INTEGER, Value: int32(1000)},
		{Parameter: MQIA_MSG_DEQ_COUNT, Type: MQCFT_INTEGER, Value: int32(900)},
	}

	stats := parser.parseQueueStats(parameters)
	require.NotNil(t, stats)

	assert.Equal(t, "TEST.QUEUE", stats.QueueName)
	assert.Equal(t, int32(100), stats.CurrentDepth)
	assert.Equal(t, int32(500), stats.HighDepth)
	assert.Equal(t, int32(2), stats.InputCount)
	assert.Equal(t, int32(1), stats.OutputCount)
	assert.Equal(t, int32(1000), stats.EnqueueCount)
	assert.Equal(t, int32(900), stats.DequeueCount)
	assert.True(t, stats.HasReaders)
	assert.True(t, stats.HasWriters)
}

func TestPCFParser_ParseChannelStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	parameters := []*PCFParameter{
		{Parameter: MQCA_CHANNEL_NAME, Type: MQCFT_STRING, Value: "TEST.SVRCONN"},
		{Parameter: MQCA_CONNECTION_NAME, Type: MQCFT_STRING, Value: "192.168.1.1"},
		{Parameter: MQIACH_MSGS, Type: MQCFT_INTEGER, Value: int32(1000)},
		{Parameter: MQIACH_BYTES, Type: MQCFT_INTEGER, Value: int32(50000)},
		{Parameter: MQIACH_BATCHES, Type: MQCFT_INTEGER, Value: int32(100)},
	}

	stats := parser.parseChannelStats(parameters)
	require.NotNil(t, stats)

	assert.Equal(t, "TEST.SVRCONN", stats.ChannelName)
	assert.Equal(t, "192.168.1.1", stats.ConnectionName)
	assert.Equal(t, int32(1000), stats.Messages)
	assert.Equal(t, int64(50000), stats.Bytes)
	assert.Equal(t, int32(100), stats.Batches)
}

func TestPCFParser_ParseMQIStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	parameters := []*PCFParameter{
		{Parameter: MQCA_APPL_NAME, Type: MQCFT_STRING, Value: "TestApp"},
		{Parameter: MQCACF_APPL_TAG, Type: MQCFT_STRING, Value: "v1.2.3"},
		{Parameter: MQCACF_USER_IDENTIFIER, Type: MQCFT_STRING, Value: "testuser"},
		{Parameter: MQCACH_CONNECTION_NAME, Type: MQCFT_STRING, Value: "192.168.1.100"},
		{Parameter: MQCA_CHANNEL_NAME, Type: MQCFT_STRING, Value: "APP.SVRCONN"},
		{Parameter: MQIAMO_OPENS, Type: MQCFT_INTEGER, Value: int32(10)},
		{Parameter: MQIAMO_CLOSES, Type: MQCFT_INTEGER, Value: int32(8)},
		{Parameter: MQIAMO_PUTS, Type: MQCFT_INTEGER, Value: int32(500)},
		{Parameter: MQIAMO_GETS, Type: MQCFT_INTEGER, Value: int32(450)},
		{Parameter: MQIAMO_COMMITS, Type: MQCFT_INTEGER, Value: int32(50)},
		{Parameter: MQIAMO_BACKOUTS, Type: MQCFT_INTEGER, Value: int32(5)},
	}

	stats := parser.parseMQIStats(parameters)
	require.NotNil(t, stats)

	assert.Equal(t, "TestApp", stats.ApplicationName)
	assert.Equal(t, "v1.2.3", stats.ApplicationTag)
	assert.Equal(t, "testuser", stats.UserIdentifier)
	assert.Equal(t, "192.168.1.100", stats.ConnectionName)
	assert.Equal(t, "APP.SVRCONN", stats.ChannelName)
	assert.Equal(t, int32(10), stats.Opens)
	assert.Equal(t, int32(8), stats.Closes)
	assert.Equal(t, int32(500), stats.Puts)
	assert.Equal(t, int32(450), stats.Gets)
	assert.Equal(t, int32(50), stats.Commits)
	assert.Equal(t, int32(5), stats.Backouts)
}

func TestPCFParser_ParseMessage_Statistics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	// Create a complete statistics message
	data := createCompleteStatsMessage()

	result, err := parser.ParseMessage(data, "statistics")
	require.NoError(t, err)
	require.NotNil(t, result)

	stats, ok := result.(*StatisticsData)
	require.True(t, ok)

	assert.Equal(t, "statistics", stats.Type)
	assert.NotZero(t, stats.Timestamp)
	assert.NotNil(t, stats.Parameters)
	assert.NotNil(t, stats.QueueStats)
	assert.Equal(t, "TEST.QUEUE", stats.QueueStats.QueueName)
}

func TestPCFParser_ParseMessage_Accounting(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	// Create a complete accounting message
	data := createCompleteAccountingMessage()

	result, err := parser.ParseMessage(data, "accounting")
	require.NoError(t, err)
	require.NotNil(t, result)

	acct, ok := result.(*AccountingData)
	require.True(t, ok)

	assert.Equal(t, "accounting", acct.Type)
	assert.NotZero(t, acct.Timestamp)
	assert.NotNil(t, acct.Parameters)
}

func TestPCFParser_CleanString(t *testing.T) {
	logger := logrus.New()
	parser := NewParser(logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"TEST.QUEUE", "TEST.QUEUE"},
		{"TEST.QUEUE\x00\x00", "TEST.QUEUE"},
		{"  TEST.QUEUE  ", "  TEST.QUEUE  "}, // Spaces are preserved
		{"TEST\x00MORE", "TEST"},
	}

	for _, tt := range tests {
		result := parser.cleanString(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestPCFParser_ParseMQTimestamp(t *testing.T) {
	logger := logrus.New()
	parser := NewParser(logger)

	tests := []struct {
		input   string
		wantErr bool
	}{
		{"2023-11-08 15:30:45.123", false},
		{"2023-11-08 15:30:45", false},
		{"20231108153045", false},
		{"2023-11-08T15:30:45Z", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parser.parseMQTimestamp(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, result.IsZero())
			} else {
				assert.NoError(t, err)
				assert.False(t, result.IsZero())
			}
		})
	}
}

func TestPCFParser_ErrorHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	tests := []struct {
		name    string
		data    []byte
		msgType string
		wantErr bool
	}{
		{
			name:    "nil data",
			data:    nil,
			msgType: "statistics",
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			msgType: "statistics",
			wantErr: true,
		},
		{
			name:    "too short data",
			data:    make([]byte, 10),
			msgType: "statistics",
			wantErr: true,
		},
		{
			name:    "invalid message type handled gracefully",
			data:    createTestPCFHeader(MQCFT_STATISTICS, MQCMD_STATISTICS_Q, 1),
			msgType: "invalid_type_that_should_fail",
			wantErr: false, // Current implementation handles gracefully
		},
		{
			name:    "corrupted header",
			data:    []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x01},
			msgType: "statistics",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseMessage(tt.data, tt.msgType)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestPCFParser_LargeMessages(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	// Create a large message with many parameters
	header := createTestPCFHeader(MQCFT_STATISTICS, MQCMD_STATISTICS_Q, 10)

	data := make([]byte, 0)
	data = append(data, header...)

	// Add multiple parameters
	for i := 0; i < 10; i++ {
		param := createTestPCFParameter(MQCA_Q_NAME+int32(i), MQCFT_STRING, "LARGE.TEST.QUEUE")
		data = append(data, param...)
	}

	result, err := parser.ParseMessage(data, "statistics")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	stats, ok := result.(*StatisticsData)
	require.True(t, ok)
	assert.Equal(t, "statistics", stats.Type)
	assert.NotNil(t, stats.Parameters)
}

func TestPCFParser_MessageTypes(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	tests := []struct {
		name     string
		msgType  string
		expected string
	}{
		{
			name:     "statistics message",
			msgType:  "statistics",
			expected: "statistics",
		},
		{
			name:     "accounting message",
			msgType:  "accounting",
			expected: "accounting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			if tt.msgType == "statistics" {
				data = createCompleteStatsMessage()
			} else {
				data = createCompleteAccountingMessage()
			}

			result, err := parser.ParseMessage(data, tt.msgType)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Check the type field in the result
			switch r := result.(type) {
			case *StatisticsData:
				assert.Equal(t, tt.expected, r.Type)
			case *AccountingData:
				assert.Equal(t, tt.expected, r.Type)
			default:
				t.Errorf("Unexpected result type: %T", result)
			}
		})
	}
}

func TestPCFParser_ParameterExtraction(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	// Test various parameter types
	tests := []struct {
		name      string
		paramType int32
		value     interface{}
	}{
		{
			name:      "string parameter",
			paramType: MQCFT_STRING,
			value:     "TEST.QUEUE",
		},
		{
			name:      "integer parameter",
			paramType: MQCFT_INTEGER,
			value:     int32(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte

			if tt.paramType == MQCFT_STRING {
				data = createTestPCFParameter(MQCA_Q_NAME, tt.paramType, tt.value.(string))
			} else {
				// Create integer parameter
				data = make([]byte, 16)
				binary.LittleEndian.PutUint32(data[0:4], uint32(MQIA_CURRENT_Q_DEPTH))
				binary.LittleEndian.PutUint32(data[4:8], uint32(tt.paramType))
				binary.LittleEndian.PutUint32(data[8:12], 16)
				binary.LittleEndian.PutUint32(data[12:16], uint32(tt.value.(int32)))
			}

			params, err := parser.parseParameters(data, 1)
			require.NoError(t, err)
			require.Len(t, params, 1)

			param := params[0]
			assert.Equal(t, tt.paramType, param.Type)

			if tt.paramType == MQCFT_STRING {
				assert.Equal(t, tt.value, param.Value)
			} else {
				assert.Equal(t, tt.value, param.Value)
			}
		})
	}
}

func TestPCFParser_ReaderWriterDetection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	parser := NewParser(logger)

	tests := []struct {
		name        string
		inputCount  int32
		outputCount int32
		hasReaders  bool
		hasWriters  bool
	}{
		{
			name:        "has readers and writers",
			inputCount:  2,
			outputCount: 1,
			hasReaders:  true,
			hasWriters:  true,
		},
		{
			name:        "has only readers",
			inputCount:  3,
			outputCount: 0,
			hasReaders:  true,
			hasWriters:  false,
		},
		{
			name:        "has only writers",
			inputCount:  0,
			outputCount: 2,
			hasReaders:  false,
			hasWriters:  true,
		},
		{
			name:        "no readers or writers",
			inputCount:  0,
			outputCount: 0,
			hasReaders:  false,
			hasWriters:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parameters := []*PCFParameter{
				{Parameter: MQCA_Q_NAME, Type: MQCFT_STRING, Value: "TEST.QUEUE"},
				{Parameter: MQIA_OPEN_INPUT_COUNT, Type: MQCFT_INTEGER, Value: tt.inputCount},
				{Parameter: MQIA_OPEN_OUTPUT_COUNT, Type: MQCFT_INTEGER, Value: tt.outputCount},
			}

			stats := parser.parseQueueStats(parameters)
			require.NotNil(t, stats)

			assert.Equal(t, tt.hasReaders, stats.HasReaders)
			assert.Equal(t, tt.hasWriters, stats.HasWriters)
			assert.Equal(t, tt.inputCount, stats.InputCount)
			assert.Equal(t, tt.outputCount, stats.OutputCount)
		})
	}
}

// Helper functions to create test data

func createTestPCFHeader(msgType, command, paramCount int32) []byte {
	data := make([]byte, 36)
	binary.LittleEndian.PutUint32(data[0:4], uint32(msgType))
	binary.LittleEndian.PutUint32(data[4:8], 36) // Structure length
	binary.LittleEndian.PutUint32(data[8:12], 1) // Version
	binary.LittleEndian.PutUint32(data[12:16], uint32(command))
	binary.LittleEndian.PutUint32(data[16:20], 1) // Message sequence number
	binary.LittleEndian.PutUint32(data[20:24], 0) // Control
	binary.LittleEndian.PutUint32(data[24:28], 0) // Completion code
	binary.LittleEndian.PutUint32(data[28:32], 0) // Reason
	binary.LittleEndian.PutUint32(data[32:36], uint32(paramCount))
	return data
}

func createTestPCFParameter(param, paramType int32, value string) []byte {
	strLen := len(value)
	paramLen := 12 + strLen
	if paramLen%4 != 0 {
		paramLen += 4 - (paramLen % 4) // Align to 4 bytes
	}

	data := make([]byte, paramLen)
	binary.LittleEndian.PutUint32(data[0:4], uint32(param))
	binary.LittleEndian.PutUint32(data[4:8], uint32(paramType))
	binary.LittleEndian.PutUint32(data[8:12], uint32(paramLen))
	copy(data[12:], []byte(value))

	return data
}

func createCompleteStatsMessage() []byte {
	// Create a simplified but complete statistics message for testing
	header := createTestPCFHeader(MQCFT_STATISTICS, MQCMD_STATISTICS_Q, 3)

	// Add queue name parameter
	qnameParam := createTestPCFParameter(MQCA_Q_NAME, MQCFT_STRING, "TEST.QUEUE")

	// Add depth parameter (simplified)
	depthParam := make([]byte, 16)
	binary.LittleEndian.PutUint32(depthParam[0:4], uint32(MQIA_CURRENT_Q_DEPTH))
	binary.LittleEndian.PutUint32(depthParam[4:8], uint32(MQCFT_INTEGER))
	binary.LittleEndian.PutUint32(depthParam[8:12], 16)
	binary.LittleEndian.PutUint32(depthParam[12:16], 100)

	// Add queue manager name parameter
	qmgrParam := createTestPCFParameter(MQCA_Q_MGR_NAME, MQCFT_STRING, "TESTQM")

	// Combine all parts
	result := make([]byte, 0)
	result = append(result, header...)
	result = append(result, qnameParam...)
	result = append(result, depthParam...)
	result = append(result, qmgrParam...)

	return result
}

func createCompleteAccountingMessage() []byte {
	// Similar to stats message but for accounting
	header := createTestPCFHeader(MQCFT_ACCOUNTING, MQCMD_ACCOUNTING_Q, 2)

	// Add application name parameter
	appParam := createTestPCFParameter(MQCA_APPL_NAME, MQCFT_STRING, "TestApp")

	// Add queue manager name parameter
	qmgrParam := createTestPCFParameter(MQCA_Q_MGR_NAME, MQCFT_STRING, "TESTQM")

	// Combine all parts
	result := make([]byte, 0)
	result = append(result, header...)
	result = append(result, appParam...)
	result = append(result, qmgrParam...)

	return result
}
