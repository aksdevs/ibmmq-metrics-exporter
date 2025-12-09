package pcf

import (
	"encoding/binary"
	"strings"

	"github.com/sirupsen/logrus"
)

// InquiryHandler sends PCF inquiry commands and parses responses
type InquiryHandler struct {
	logger *logrus.Logger
}

// NewInquiryHandler creates a new inquiry handler
func NewInquiryHandler(logger *logrus.Logger) *InquiryHandler {
	return &InquiryHandler{
		logger: logger,
	}
}

// QueueHandleDetails represents a single queue handle from PCF inquiry
type QueueHandleDetails struct {
	QueueName      string
	ApplicationTag string // e.g., "el\bin\producer-consumer.exe"
	ChannelName    string // e.g., "APP1.SVRCONN"
	ConnectionName string // e.g., "127.0.0.1"
	UserID         string // e.g., "atulk@DESKTOP-2G7OVO3"
	ProcessID      int32  // e.g., 24600
	InputMode      string // "INPUT", "SHARED", "NO"
	OutputMode     string // "OUTPUT", "YES", "NO"
}

// BuildInquireConnectionCmd builds a PCF INQUIRE_Q command
// This queries queue details that may include connection info
// We use MQCMD_INQUIRE_Q (code 3) which is more reliable remotely
func (h *InquiryHandler) BuildInquireConnectionCmd(queueName string) []byte {
	// PCF Command Format (IBM MQ MQCMD_INQUIRE_Q = 3)
	// Parameters:
	// 1. MQCA_Q_NAME (2016) - Queue name to query

	buf := make([]byte, 0, 512)

	// PCF Header (40 bytes - MQCFH structure)
	buf = appendInt32BE(buf, 11) // MQCFT_COMMAND_XR - command with reply expected

	// StrucLength (40 bytes for the header)
	buf = appendInt32BE(buf, 40)

	// Version (MQCFH_VERSION_3 = 3)
	buf = appendInt32BE(buf, 3)

	// Command (MQCMD_INQUIRE_Q = 3) - simpler, more reliable
	buf = appendInt32BE(buf, 3)

	// MsgSeqNumber
	buf = appendInt32BE(buf, 1)

	// Control (MQCFC_LAST = 1)
	buf = appendInt32BE(buf, 1)

	// CompCode
	buf = appendInt32BE(buf, 0)

	// Reason
	buf = appendInt32BE(buf, 0)

	// ParameterCount
	buf = appendInt32BE(buf, 1) // Just the queue name

	// Reserved
	buf = appendInt32BE(buf, 0)

	// Parameter 1: Queue Name (MQCA_Q_NAME = 2016)
	paramStart := len(buf)

	// Type (MQCFT_STRING = 4)
	buf = appendInt32BE(buf, 4)

	// StrucLength (will be calculated)
	paramLenPos := len(buf)
	buf = appendInt32BE(buf, 0)

	// Parameter (MQCA_Q_NAME = 2016)
	buf = appendInt32BE(buf, 2016)

	// CodedCharSetId (system default = 0)
	buf = appendInt32BE(buf, 0)

	// StringLength
	qnameLen := len(queueName)
	buf = appendInt32BE(buf, int32(qnameLen))

	// String data (padded to 4-byte boundary)
	buf = append(buf, []byte(queueName)...)
	padding := (4 - (qnameLen % 4)) % 4
	for i := 0; i < padding; i++ {
		buf = append(buf, 0x00)
	}

	// Update parameter length
	paramLen := len(buf) - paramStart
	binary.BigEndian.PutUint32(buf[paramLenPos:], uint32(paramLen))

	h.logger.WithFields(map[string]interface{}{
		"queue_name": queueName,
		"msg_size":   len(buf),
		"command":    3, // MQCMD_INQUIRE_Q
	}).Debug("Built INQUIRE_Q PCF command")

	return buf
}

// BuildInquireQueueStatusCmd builds a PCF INQUIRE_QUEUE_STATUS command
// This queries queue handle details similar to: DIS QS(queue_name) TYPE(HANDLE) ALL
func (h *InquiryHandler) BuildInquireQueueStatusCmd(queueName string) []byte {
	// PCF Command Format (IBM MQ MQCMD_INQUIRE_Q_STATUS = 34)
	// Parameters needed:
	// 1. MQCA_Q_NAME (2016) - Queue name
	// 2. MQIACF_Q_STATUS_TYPE (1238) with value MQIACF_Q_STATUS_HANDLE (2)

	buf := make([]byte, 0, 512)

	// PCF Header (40 bytes - MQCFH structure)
	// Offset 0-3: Type (MQCFT_COMMAND_XR = 11 for command with reply expected)
	buf = appendInt32BE(buf, 11) // MQCFT_COMMAND_XR

	// Offset 4-7: StrucLength (40 bytes for the header)
	buf = appendInt32BE(buf, 40)

	// Offset 8-11: Version (MQCFH_VERSION_3 = 3)
	buf = appendInt32BE(buf, 3)

	// Offset 12-15: Command (MQCMD_INQUIRE_Q_STATUS = 34)
	buf = appendInt32BE(buf, 34)

	// Offset 16-19: MsgSeqNumber
	buf = appendInt32BE(buf, 1)

	// Offset 20-23: Control (MQCFC_LAST = 1)
	buf = appendInt32BE(buf, 1)

	// Offset 24-27: CompCode
	buf = appendInt32BE(buf, 0)

	// Offset 28-31: Reason
	buf = appendInt32BE(buf, 0)

	// Offset 32-35: ParameterCount
	buf = appendInt32BE(buf, 2) // Will have 2 parameters

	// Offset 36-39: Padding/reserved
	buf = appendInt32BE(buf, 0)

	// Parameter 1: Queue Name (MQCA_Q_NAME = 2016)
	// MQCFST structure for string parameter
	paramStart := len(buf)

	// Type (MQCFT_STRING = 4)
	buf = appendInt32BE(buf, 4)

	// StrucLength (will be calculated)
	paramLenPos := len(buf)
	buf = appendInt32BE(buf, 0)

	// Parameter (MQCA_Q_NAME = 2016)
	buf = appendInt32BE(buf, 2016)

	// CodedCharSetId (system default = 0)
	buf = appendInt32BE(buf, 0)

	// StringLength
	qnameLen := len(queueName)
	buf = appendInt32BE(buf, int32(qnameLen))

	// String data (padded to 4-byte boundary)
	buf = append(buf, []byte(queueName)...)
	padding := (4 - (qnameLen % 4)) % 4
	for i := 0; i < padding; i++ {
		buf = append(buf, 0x00)
	}

	// Update parameter length
	paramLen := len(buf) - paramStart
	binary.BigEndian.PutUint32(buf[paramLenPos:], uint32(paramLen))

	// Parameter 2: Status Type (MQIACF_Q_STATUS_TYPE = 1238)
	// MQCFIN structure for integer parameter
	paramStart = len(buf)

	// Type (MQCFT_INTEGER = 3)
	buf = appendInt32BE(buf, 3)

	// StrucLength (16 bytes for integer parameter)
	buf = appendInt32BE(buf, 16)

	// Parameter (MQIACF_Q_STATUS_TYPE = 1238)
	buf = appendInt32BE(buf, 1238)

	// Value (MQIACF_Q_STATUS_HANDLE = 2)
	buf = appendInt32BE(buf, 2)

	h.logger.WithFields(map[string]interface{}{
		"queue_name":  queueName,
		"msg_size":    len(buf),
		"command":     34, // MQCMD_INQUIRE_Q_STATUS
		"status_type": 2,  // MQIACF_Q_STATUS_HANDLE
	}).Debug("Built INQUIRE_Q_STATUS PCF command for handle inquiry")

	return buf
}

// ParseQueueStatusResponse parses a PCF response containing handle details
// Returns a list of queue handles with detailed information
func (h *InquiryHandler) ParseQueueStatusResponse(data []byte) []*QueueHandleDetails {
	handles := make([]*QueueHandleDetails, 0)

	if len(data) < 36 {
		h.logger.WithField("data_len", len(data)).Debug("Response too short for PCF header")
		return handles
	}

	// Parse PCF Header (MQCFH)
	// Offset 0-3: Type
	msgType := binary.BigEndian.Uint32(data[0:4])

	// Offset 4-7: StrucLength
	strucLen := binary.BigEndian.Uint32(data[4:8])

	// Offset 8-11: Version
	version := binary.BigEndian.Uint32(data[8:12])

	// Offset 12-15: Command
	command := binary.BigEndian.Uint32(data[12:16])

	// Offset 28-31: CompCode
	compCode := binary.BigEndian.Uint32(data[28:32])

	// Offset 32-35: Reason
	reason := binary.BigEndian.Uint32(data[32:36])

	// Offset 36-39: ParameterCount
	if len(data) < 40 {
		return handles
	}
	paramCount := binary.BigEndian.Uint32(data[36:40])

	h.logger.WithFields(map[string]interface{}{
		"msg_type":    msgType,
		"struc_len":   strucLen,
		"version":     version,
		"command":     command,
		"comp_code":   compCode,
		"reason":      reason,
		"param_count": paramCount,
	}).Debug("Parsed PCF response header")

	// Check for errors
	if compCode != 0 {
		h.logger.WithFields(map[string]interface{}{
			"comp_code": compCode,
			"reason":    reason,
		}).Warn("PCF command returned error")
		return handles
	}

	// Parse parameters starting at offset 40
	offset := 40
	currentHandle := &QueueHandleDetails{}
	inGroup := false

	for offset < len(data) && offset < int(strucLen) {
		if offset+8 > len(data) {
			break
		}

		paramType := binary.BigEndian.Uint32(data[offset : offset+4])
		paramStrucLen := binary.BigEndian.Uint32(data[offset+4 : offset+8])

		if paramStrucLen == 0 || offset+int(paramStrucLen) > len(data) {
			break
		}

		paramData := data[offset+8 : offset+int(paramStrucLen)]

		// Handle parameter based on type
		switch paramType {
		case 20: // MQCFT_GROUP - Start of a new handle group
			if inGroup && currentHandle.QueueName != "" {
				handles = append(handles, currentHandle)
			}
			currentHandle = &QueueHandleDetails{}
			inGroup = true
			h.logger.Debug("Started new handle group")

		case 4: // MQCFT_STRING
			h.parseStringParameter(paramData, currentHandle)

		case 3: // MQCFT_INTEGER
			h.parseIntegerParameter(paramData, currentHandle)
		}

		offset += int(paramStrucLen)
	}

	// Add last handle if any
	if inGroup && currentHandle.QueueName != "" {
		handles = append(handles, currentHandle)
	}

	h.logger.WithField("handles_found", len(handles)).Info("Parsed queue handle details from PCF response")
	return handles
}

func (h *InquiryHandler) parseStringParameter(paramData []byte, handle *QueueHandleDetails) {
	if len(paramData) < 12 {
		return
	}

	// MQCFST structure
	// Offset 0-3: Parameter ID
	// Offset 4-7: CodedCharSetId
	// Offset 8-11: StringLength
	// Offset 12+: String data

	paramID := binary.BigEndian.Uint32(paramData[0:4])
	strLen := binary.BigEndian.Uint32(paramData[8:12])

	if 12+int(strLen) > len(paramData) {
		return
	}

	strValue := strings.TrimRight(string(paramData[12:12+strLen]), "\x00 ")

	switch paramID {
	case 2016: // MQCA_Q_NAME
		handle.QueueName = strValue
	case 3501: // MQCACH_CHANNEL_NAME
		handle.ChannelName = strValue
	case 3502: // MQCACH_CONNECTION_NAME
		handle.ConnectionName = strValue
	case 2024: // MQCA_APPL_NAME
		// Application name
	case 2549: // MQCACF_APPL_TAG
		handle.ApplicationTag = strValue
	case 2046: // MQCA_USER_ID
		handle.UserID = strValue
	case 1238: // Input/Output mode indicators
		// Handle INPUT/OUTPUT mode
		if strings.Contains(strings.ToUpper(strValue), "INPUT") {
			handle.InputMode = strValue
		} else if strings.Contains(strings.ToUpper(strValue), "OUTPUT") {
			handle.OutputMode = strValue
		}
	}
}

func (h *InquiryHandler) parseIntegerParameter(paramData []byte, handle *QueueHandleDetails) {
	if len(paramData) < 8 {
		return
	}

	// MQCFIN structure
	// Offset 0-3: Parameter ID
	// Offset 4-7: Value

	paramID := binary.BigEndian.Uint32(paramData[0:4])
	intValue := int32(binary.BigEndian.Uint32(paramData[4:8]))

	switch paramID {
	case 3002: // MQIACF_PROCESS_ID
		handle.ProcessID = intValue
	case 1238: // MQIACF_Q_STATUS_TYPE or similar - may indicate mode
		// Check for input/output indicators
	case 1411: // MQIACF_OPEN_INPUT_TYPE
		if intValue > 0 {
			handle.InputMode = "INPUT"
		}
	case 1412: // MQIACF_OPEN_OUTPUT
		if intValue > 0 {
			handle.OutputMode = "OUTPUT"
		}
	}
}

// appendInt32BE appends an int32 value in big-endian format
func appendInt32BE(buf []byte, value int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(value))
	return append(buf, b...)
}
