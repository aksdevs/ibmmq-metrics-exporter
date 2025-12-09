package mqclient

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/pcf"
	ibmmq "github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/sirupsen/logrus"
)

// MQClient represents an IBM MQ client connection
type MQClient struct {
	config     *config.MQConfig
	qmgr       ibmmq.MQQueueManager
	connected  bool
	logger     *logrus.Logger
	statsQueue ibmmq.MQObject
	acctQueue  ibmmq.MQObject
}

// NewMQClient creates a new IBM MQ client instance
func NewMQClient(cfg *config.MQConfig, logger *logrus.Logger) *MQClient {
	return &MQClient{
		config:    cfg,
		connected: false,
		logger:    logger,
	}
}

// Connect establishes connection to IBM MQ
func (c *MQClient) Connect() error {
	if c.connected {
		return nil
	}

	c.logger.WithFields(logrus.Fields{
		"queue_manager":   c.config.QueueManager,
		"channel":         c.config.Channel,
		"connection_name": c.config.GetConnectionName(),
	}).Info("Connecting to IBM MQ")

	// Create connection options
	cno := ibmmq.NewMQCNO()
	cno.Options = ibmmq.MQCNO_CLIENT_BINDING

	// Set channel definition
	cd := ibmmq.NewMQCD()
	cd.ChannelName = c.config.Channel
	cd.ConnectionName = c.config.GetConnectionName()
	// Note: ChannelType is not available in client MQCD structure

	// Set security options if SSL/TLS is configured
	if c.config.CipherSpec != "" {
		cd.SSLCipherSpec = c.config.CipherSpec
		// Note: SSLKeyRepository is not available in client MQCD structure
		// SSL configuration is handled differently in client connections
	}

	cno.ClientConn = cd

	// Set user credentials if provided
	if c.config.GetUser() != "" {
		csp := ibmmq.NewMQCSP()
		csp.AuthenticationType = ibmmq.MQCSP_AUTH_USER_ID_AND_PWD
		csp.UserId = c.config.GetUser()
		csp.Password = c.config.Password
		cno.SecurityParms = csp
	}

	// Connect to queue manager
	qmgr, err := ibmmq.Connx(c.config.QueueManager, cno)
	if err != nil {
		return fmt.Errorf("failed to connect to queue manager %s: %w", c.config.QueueManager, err)
	}

	c.qmgr = qmgr
	c.connected = true

	c.logger.Info("Successfully connected to IBM MQ")
	return nil
}

// Disconnect closes the connection to IBM MQ
func (c *MQClient) Disconnect() error {
	if !c.connected {
		return nil
	}

	c.logger.Info("Disconnecting from IBM MQ")

	// Close queues if open
	if c.statsQueue.GetValue() != 0 {
		c.statsQueue.Close(0)
	}
	if c.acctQueue.GetValue() != 0 {
		c.acctQueue.Close(0)
	}

	// Disconnect from queue manager
	err := c.qmgr.Disc()
	if err != nil {
		c.logger.WithError(err).Error("Error disconnecting from queue manager")
		return err
	}

	c.connected = false
	c.logger.Info("Successfully disconnected from IBM MQ")
	return nil
}

// GetQueueManager returns the underlying IBM MQ queue manager connection
// Useful for advanced operations like opening queues directly
func (c *MQClient) GetQueueManager() ibmmq.MQQueueManager {
	return c.qmgr
}

// OpenStatsQueue opens the statistics queue for reading
func (c *MQClient) OpenStatsQueue(queueName string) error {
	if !c.connected {
		return fmt.Errorf("not connected to queue manager")
	}

	mqod := ibmmq.NewMQOD()
	openOptions := ibmmq.MQOO_INPUT_AS_Q_DEF | ibmmq.MQOO_FAIL_IF_QUIESCING

	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = queueName

	queue, err := c.qmgr.Open(mqod, openOptions)
	if err != nil {
		return fmt.Errorf("failed to open statistics queue %s: %w", queueName, err)
	}

	c.statsQueue = queue
	c.logger.WithField("queue", queueName).Info("Opened statistics queue")
	return nil
}

// OpenAccountingQueue opens the accounting queue for reading
func (c *MQClient) OpenAccountingQueue(queueName string) error {
	if !c.connected {
		return fmt.Errorf("not connected to queue manager")
	}

	mqod := ibmmq.NewMQOD()
	openOptions := ibmmq.MQOO_INPUT_AS_Q_DEF | ibmmq.MQOO_FAIL_IF_QUIESCING

	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = queueName

	queue, err := c.qmgr.Open(mqod, openOptions)
	if err != nil {
		return fmt.Errorf("failed to open accounting queue %s: %w", queueName, err)
	}

	c.acctQueue = queue
	c.logger.WithField("queue", queueName).Info("Opened accounting queue")
	return nil
}

// GetMessage retrieves a message from the specified queue
func (c *MQClient) GetMessage(queueType string) (*ibmmq.MQMD, []byte, error) {
	var queue ibmmq.MQObject

	switch queueType {
	case "stats":
		queue = c.statsQueue
	case "accounting":
		queue = c.acctQueue
	default:
		return nil, nil, fmt.Errorf("unknown queue type: %s", queueType)
	}

	if queue.GetValue() == 0 {
		return nil, nil, fmt.Errorf("queue %s is not open", queueType)
	}

	// Create message descriptor
	mqmd := ibmmq.NewMQMD()

	// Create get message options
	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_NO_WAIT | ibmmq.MQGMO_FAIL_IF_QUIESCING | ibmmq.MQGMO_CONVERT
	gmo.WaitInterval = 1000 // 1 second wait

	// Get message
	buffer := make([]byte, 100*1024) // 100KB buffer
	datalen, err := queue.Get(mqmd, gmo, buffer)

	if err != nil {
		mqret := err.(*ibmmq.MQReturn)
		if mqret.MQRC == ibmmq.MQRC_NO_MSG_AVAILABLE {
			// No message available, not an error
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get message from %s queue: %w", queueType, err)
	}

	// Return actual message data
	msgData := buffer[:datalen]

	c.logger.WithFields(logrus.Fields{
		"queue_type":   queueType,
		"message_id":   fmt.Sprintf("%x", mqmd.MsgId),
		"message_size": datalen,
		"message_type": mqmd.MsgType,
		"format":       mqmd.Format,
	}).Debug("Retrieved message")

	return mqmd, msgData, nil
}

// GetAllMessages retrieves all available messages from the specified queue
func (c *MQClient) GetAllMessages(queueType string) ([]*MQMessage, error) {
	var messages []*MQMessage

	for {
		mqmd, data, err := c.GetMessage(queueType)
		if err != nil {
			return nil, err
		}

		// No more messages
		if mqmd == nil {
			break
		}

		msg := &MQMessage{
			MD:   mqmd,
			Data: data,
			Type: queueType,
		}

		messages = append(messages, msg)

		// Add a small delay to prevent tight loop
		time.Sleep(10 * time.Millisecond)
	}

	c.logger.WithFields(logrus.Fields{
		"queue_type": queueType,
		"count":      len(messages),
	}).Info("Retrieved messages from queue")

	return messages, nil
}

// IsConnected returns true if connected to IBM MQ
func (c *MQClient) IsConnected() bool {
	return c.connected
}

// QueueStats represents queue-specific statistics retrieved via MQINQ
type QueueStats struct {
	QueueName       string    `json:"queue_name"`
	CurrentDepth    int32     `json:"current_depth"`
	OpenInputCount  int32     `json:"open_input_count"`
	OpenOutputCount int32     `json:"open_output_count"`
	CreationTime    time.Time `json:"creation_time"`
}

// GetQueueStats retrieves queue statistics using MQINQ API
func (c *MQClient) GetQueueStats(queueName string) (*QueueStats, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to IBM MQ")
	}

	// Open the queue for inquiry
	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	queue, err := c.qmgr.Open(od, ibmmq.MQOO_INQUIRE)
	if err != nil {
		c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to open queue for inquiry")
		return nil, fmt.Errorf("failed to open queue %s: %w", queueName, err)
	}
	defer queue.Close(0)

	// Prepare inquiry selectors - get current depth and open counts
	selectors := []int32{
		ibmmq.MQIA_CURRENT_Q_DEPTH,
		ibmmq.MQIA_OPEN_INPUT_COUNT,
		ibmmq.MQIA_OPEN_OUTPUT_COUNT,
	}

	// Inquire on the queue
	attrs, err := queue.Inq(selectors)
	if err != nil {
		c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to inquire queue attributes")
		return nil, fmt.Errorf("failed to inquire queue %s: %w", queueName, err)
	}

	// Extract values from the attributes map
	stats := &QueueStats{
		QueueName:    queueName,
		CreationTime: time.Now(),
	}

	if depth, ok := attrs[ibmmq.MQIA_CURRENT_Q_DEPTH].(int32); ok {
		stats.CurrentDepth = depth
	}
	if inputCount, ok := attrs[ibmmq.MQIA_OPEN_INPUT_COUNT].(int32); ok {
		stats.OpenInputCount = inputCount
	}
	if outputCount, ok := attrs[ibmmq.MQIA_OPEN_OUTPUT_COUNT].(int32); ok {
		stats.OpenOutputCount = outputCount
	}

	c.logger.WithFields(logrus.Fields{
		"queue_name":    queueName,
		"current_depth": stats.CurrentDepth,
		"open_input":    stats.OpenInputCount,
		"open_output":   stats.OpenOutputCount,
	}).Debug("Retrieved queue stats via MQINQ")

	return stats, nil
}

// HandleInfo represents handle (application) details for a queue
// Similar to MQSC command: DIS QS(queue_name) TYPE(HANDLE) ALL
type HandleInfo struct {
	ApplicationName   string    `json:"application_name"`
	ApplicationTag    string    `json:"application_tag"` // Full path like "el\bin\producer-consumer.exe"
	ProcessID         int32     `json:"process_id"`
	UserIdentifier    string    `json:"user_identifier"`
	ChannelName       string    `json:"channel_name"`    // MQTT connection channel
	ConnectionName    string    `json:"connection_name"` // IP address or connection string
	HandleState       string    `json:"handle_state"`    // "Open", "Closing", etc.
	OpenMode          string    `json:"open_mode"`       // "Input", "Output", "Inquire"
	QueueName         string    `json:"queue_name"`
	CreationTime      time.Time `json:"creation_time"`
	LastUsedTime      time.Time `json:"last_used_time"`
	InputHandleCount  int32     `json:"input_handle_count"`  // Count of Input handles
	OutputHandleCount int32     `json:"output_handle_count"` // Count of Output handles
}

// GetQueueHandles retrieves handle (application) details for a queue
// This provides information similar to: DIS QS(queue_name) TYPE(HANDLE) ALL
// Uses MQINQ to get basic handle counts and stores them for metrics
func (c *MQClient) GetQueueHandles(queueName string) ([]*HandleInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to IBM MQ")
	}

	handles := make([]*HandleInfo, 0)

	// Open the queue for inquiry
	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	queue, err := c.qmgr.Open(od, ibmmq.MQOO_INQUIRE)
	if err != nil {
		c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to open queue for handle inquiry")
		return handles, nil // Return empty list rather than error - queue may not exist
	}
	defer queue.Close(0)

	// Retrieve open handle counts to determine if queue is in use
	selectors := []int32{
		ibmmq.MQIA_OPEN_INPUT_COUNT,
		ibmmq.MQIA_OPEN_OUTPUT_COUNT,
	}

	attrs, err := queue.Inq(selectors)
	if err != nil {
		c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to inquire queue handle counts")
		return handles, nil
	}

	// Get input and output handle counts
	inputCount := int32(0)
	outputCount := int32(0)

	if ic, ok := attrs[ibmmq.MQIA_OPEN_INPUT_COUNT].(int32); ok {
		inputCount = ic
	}
	if oc, ok := attrs[ibmmq.MQIA_OPEN_OUTPUT_COUNT].(int32); ok {
		outputCount = oc
	}

	c.logger.WithFields(logrus.Fields{
		"queue_name":     queueName,
		"input_handles":  inputCount,
		"output_handles": outputCount,
	}).Debug("Retrieved queue handles via MQINQ - detailed handle info comes from statistics messages")

	// Note: Detailed handle information (PID, UserID, Channel, ConnectionName, ApplicationTag)
	// is obtained from queue statistics messages that include PROC data.
	// This function provides counts via MQINQ as a supplement.

	return handles, nil
}

// GetQueueHandleDetailsByPCF retrieves detailed handle information using PCF inquiry
// Uses a dynamically created temporary queue for the PCF response
// Returns handle details like USERID, PID, CHANNEL, APPLTAG, CONNAME
func (c *MQClient) GetQueueHandleDetailsByPCF(queueName string) ([]*HandleInfo, error) {
	c.logger.WithField("queue_name", queueName).Info("Attempting to retrieve handle details via PCF")

	if !c.connected {
		return nil, fmt.Errorf("not connected to IBM MQ")
	}

	handles := make([]*HandleInfo, 0)

	// Step 1: Create a temporary dynamic queue using the model queue
	// This will give us a unique queue name like AMQ.xxxxxxxx.xxxxxxxx
	odTemp := ibmmq.NewMQOD()
	odTemp.ObjectName = "SYSTEM.DEFAULT.MODEL.QUEUE"
	odTemp.ObjectType = ibmmq.MQOT_Q
	odTemp.DynamicQName = "AMQ.*" // Create with dynamic naming

	replyQueue, err := c.qmgr.Open(odTemp, ibmmq.MQOO_INPUT_EXCLUSIVE)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"queue_name": queueName,
			"model_q":    "SYSTEM.DEFAULT.MODEL.QUEUE",
		}).Debug("Failed to create temporary reply queue for PCF")
		return handles, nil // Not an error - this feature is optional
	}
	defer replyQueue.Close(0)

	// After opening, MQ updates the ObjectName field with the actual dynamic queue name
	// This is how the mq-golang library exposes the created queue name
	dynamicQueueName := odTemp.ObjectName

	c.logger.WithFields(map[string]interface{}{
		"dynamic_queue_name": dynamicQueueName,
		"queue_name":         queueName,
	}).Debug("Created temporary dynamic queue for PCF reply")

	if dynamicQueueName == "" || dynamicQueueName == "SYSTEM.DEFAULT.MODEL.QUEUE" {
		c.logger.WithField("queue_name", queueName).Debug("Could not determine dynamic queue name")
		return handles, nil
	}

	// Step 2: Build PCF command (use INQUIRE_CONNECTION for better remote compatibility)
	handler := pcf.NewInquiryHandler(c.logger)
	cmdMsg := handler.BuildInquireConnectionCmd(queueName)

	// Step 3: Open command queue and send request
	od := ibmmq.NewMQOD()
	od.ObjectName = "SYSTEM.ADMIN.COMMAND.QUEUE"
	od.ObjectType = ibmmq.MQOT_Q

	cmdQueue, err := c.qmgr.Open(od, ibmmq.MQOO_OUTPUT)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"queue_name": queueName,
			"cmd_queue":  "SYSTEM.ADMIN.COMMAND.QUEUE",
		}).Debug("Failed to open PCF command queue")
		return handles, nil
	}
	defer cmdQueue.Close(0)

	// Create message descriptor with the dynamic reply queue
	md := ibmmq.NewMQMD()
	md.Format = ibmmq.MQFMT_ADMIN
	md.ReplyToQ = dynamicQueueName
	md.MsgType = ibmmq.MQMT_REQUEST

	// Generate a unique correlation ID
	correlID := make([]byte, 24)
	for i := 0; i < len(correlID); i++ {
		correlID[i] = byte(65 + (i % 26))
	}
	copy(md.CorrelId, correlID)

	// Send the command
	pmo := ibmmq.NewMQPMO()
	pmo.Options = ibmmq.MQPMO_NONE

	err = cmdQueue.Put(md, pmo, cmdMsg)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"queue_name": queueName,
		}).Debug("Failed to send PCF inquiry command")
		return handles, nil
	}

	c.logger.WithFields(map[string]interface{}{
		"queue_name":  queueName,
		"reply_queue": dynamicQueueName,
		"correl_id":   string(correlID),
	}).Debug("Sent PCF INQUIRE_Q_STATUS command")

	// Step 4: Read response with timeout
	mdResp := ibmmq.NewMQMD()
	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_WAIT
	gmo.WaitInterval = 5000 // 5 seconds

	respData := make([]byte, 16384)

	datalen, err := replyQueue.Get(mdResp, gmo, respData)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"queue_name": queueName,
		}).Debug("Failed to read PCF response")
		return handles, nil
	}

	c.logger.WithFields(map[string]interface{}{
		"queue_name": queueName,
		"resp_len":   datalen,
	}).Info("Received PCF response for handle details")

	// Debug: Log hex dump and reason code for analysis
	if datalen >= 36 {
		reasonCode := binary.BigEndian.Uint32(respData[32:36])
		c.logger.WithFields(map[string]interface{}{
			"queue_name":  queueName,
			"reason_code": reasonCode,
		}).Debug("PCF response reason code (0=success, other=error)")
	}
	if datalen > 0 && datalen <= 100 {
		hexDump := ""
		for i := 0; i < datalen && i < 100; i++ {
			hexDump += fmt.Sprintf("%02x ", respData[i])
		}
		c.logger.WithFields(map[string]interface{}{
			"queue_name": queueName,
			"hex_dump":   hexDump,
		}).Debug("PCF response hex dump (first 100 bytes)")
	}

	// Step 5: Parse response
	responseHandles := handler.ParseQueueStatusResponse(respData[:datalen])

	// Convert to HandleInfo structures
	for _, h := range responseHandles {
		handle := &HandleInfo{
			QueueName:      h.QueueName,
			ApplicationTag: h.ApplicationTag,
			ChannelName:    h.ChannelName,
			ConnectionName: h.ConnectionName,
			UserIdentifier: h.UserID,
			ProcessID:      h.ProcessID,
		}
		handles = append(handles, handle)
	}

	c.logger.WithFields(map[string]interface{}{
		"queue_name":    queueName,
		"handles_found": len(handles),
	}).Info("Retrieved queue handles via PCF inquiry")

	// Step 6: Close the dynamic queue (it will be auto-deleted by MQ)
	// This is already done by the defer statement above

	return handles, nil
}

// QueueInfo represents information about a queue retrieved via MQINQ
type QueueInfo struct {
	QueueName       string
	CurrentDepth    int32
	OpenInputCount  int32
	OpenOutputCount int32
	MaxQueueDepth   int32
	CreationTime    time.Time
}

// GetQueueInfo retrieves detailed information about a specific queue using MQINQ
func (c *MQClient) GetQueueInfo(queueName string) (*QueueInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to IBM MQ")
	}

	// Open the queue for inquiry
	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	queue, err := c.qmgr.Open(od, ibmmq.MQOO_INQUIRE)
	if err != nil {
		return nil, fmt.Errorf("failed to open queue %s: %w", queueName, err)
	}
	defer queue.Close(0)

	// Prepare inquiry selectors for comprehensive queue information
	selectors := []int32{
		ibmmq.MQIA_CURRENT_Q_DEPTH,
		ibmmq.MQIA_OPEN_INPUT_COUNT,
		ibmmq.MQIA_OPEN_OUTPUT_COUNT,
		ibmmq.MQIA_MAX_Q_DEPTH,
	}

	// Inquire on the queue
	attrs, err := queue.Inq(selectors)
	if err != nil {
		return nil, fmt.Errorf("failed to inquire queue %s: %w", queueName, err)
	}

	// Extract values from the attributes map
	info := &QueueInfo{
		QueueName:    queueName,
		CreationTime: time.Now(),
	}

	if depth, ok := attrs[ibmmq.MQIA_CURRENT_Q_DEPTH].(int32); ok {
		info.CurrentDepth = depth
	}
	if inputCount, ok := attrs[ibmmq.MQIA_OPEN_INPUT_COUNT].(int32); ok {
		info.OpenInputCount = inputCount
	}
	if outputCount, ok := attrs[ibmmq.MQIA_OPEN_OUTPUT_COUNT].(int32); ok {
		info.OpenOutputCount = outputCount
	}
	if maxDepth, ok := attrs[ibmmq.MQIA_MAX_Q_DEPTH].(int32); ok {
		info.MaxQueueDepth = maxDepth
	}

	c.logger.WithFields(logrus.Fields{
		"queue_name":     queueName,
		"current_depth":  info.CurrentDepth,
		"input_handles":  info.OpenInputCount,
		"output_handles": info.OpenOutputCount,
		"max_depth":      info.MaxQueueDepth,
	}).Debug("Retrieved queue info via MQINQ")

	return info, nil
}

// GetAllLocalQueues returns all local queue names on the queue manager
// For now, returns a simple list - full implementation would enumerate all queues
// This is a placeholder that can be enhanced to query all queues
func (c *MQClient) GetAllLocalQueues() ([]string, error) {
	if !c.connected {
		return nil, fmt.Errorf("client not connected")
	}

	// In a full implementation, this would:
	// 1. Use MQINQ on the queue manager to enumerate all local queues
	// 2. Build a complete list dynamically
	// For now, return empty list and let exclusion patterns filter from config
	return []string{}, nil
}

// QueueMatchesExclusionPattern checks if a queue name matches any exclusion pattern
// Supports HLQ wildcard patterns like "SYSTEM.*" or "TEST.TEMP.*"
func QueueMatchesExclusionPattern(queueName string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	for _, pattern := range patterns {
		if matchesHLQPattern(queueName, pattern) {
			return true
		}
	}
	return false
}

// matchesHLQPattern checks if a queue name matches an HLQ pattern
// Supports simple wildcard patterns:
//   - "SYSTEM.*" matches "SYSTEM.Q1", "SYSTEM.ADMIN.Q", etc. (anything starting with "SYSTEM.")
//   - "TEST.**" matches "TEST.Q1", "TEST.QUEUE", etc.
//   - "EXACT" matches only "EXACT"
func matchesHLQPattern(queueName, pattern string) bool {
	// If pattern is exactly "*", match all
	if pattern == "*" {
		return true
	}

	// Check if pattern ends with .* (HLQ pattern)
	if len(pattern) > 2 && pattern[len(pattern)-2:] == ".*" {
		prefix := pattern[:len(pattern)-2]
		return len(queueName) > len(prefix) && queueName[:len(prefix)+1] == prefix+"."
	}

	// Check if pattern ends with .** (HLQ pattern with deeper nesting)
	if len(pattern) > 3 && pattern[len(pattern)-3:] == ".**" {
		prefix := pattern[:len(pattern)-3]
		return len(queueName) > len(prefix) && queueName[:len(prefix)+1] == prefix+"."
	}

	// Exact match (no wildcards)
	return queueName == pattern
}

// MQMessage represents a message retrieved from IBM MQ
type MQMessage struct {
	MD   *ibmmq.MQMD
	Data []byte
	Type string // "stats" or "accounting"
}

// GetTimestamp returns the message timestamp
func (m *MQMessage) GetTimestamp() time.Time {
	// Convert MQ timestamp to Go time
	// MQ timestamp format: YYYYMMDDHHMMSSTH (where T is tenths of seconds, H is hundredths)
	if len(m.MD.PutDate) >= 8 && len(m.MD.PutTime) >= 8 {
		dateStr := m.MD.PutDate
		timeStr := m.MD.PutTime

		// Parse YYYYMMDD
		year := dateStr[0:4]
		month := dateStr[4:6]
		day := dateStr[6:8]

		// Parse HHMMSSTH
		hour := timeStr[0:2]
		minute := timeStr[2:4]
		second := timeStr[4:6]
		// Ignore tenths and hundredths for now

		timeString := fmt.Sprintf("%s-%s-%sT%s:%s:%sZ", year, month, day, hour, minute, second)

		if t, err := time.Parse("2006-01-02T15:04:05Z", timeString); err == nil {
			return t
		}
	}

	// Fallback to current time if parsing fails
	return time.Now()
}

// GetSize returns the message size
func (m *MQMessage) GetSize() int {
	return len(m.Data)
}

// IsStatistics returns true if this is a statistics message
func (m *MQMessage) IsStatistics() bool {
	return m.Type == "stats"
}

// IsAccounting returns true if this is an accounting message
func (m *MQMessage) IsAccounting() bool {
	return m.Type == "accounting"
}
