package mqclient

import (
	"fmt"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
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
	ApplicationName string    `json:"application_name"`
	ProcessID       int32     `json:"process_id"`
	UserIdentifier  string    `json:"user_identifier"`
	ConnectionName  string    `json:"connection_name"`
	HandleState     string    `json:"handle_state"` // "Open", "Closing", etc.
	OpenMode        string    `json:"open_mode"`    // "Input", "Output", "Inquire"
	QueueName       string    `json:"queue_name"`
	CreationTime    time.Time `json:"creation_time"`
	LastUsedTime    time.Time `json:"last_used_time"`
}

// GetQueueHandles retrieves handle (application) details for a queue
// This provides information similar to: DIS QS(queue_name) TYPE(HANDLE) ALL
// Note: In Go MQ client, handle details are exposed through queue attributes
// We retrieve process info by inspecting open handles and their attributes
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

	// Create handle info records for each open handle
	// Note: The Go MQ client library does not expose individual handle details directly
	// However, we can infer handle presence and mode from the open counts
	totalHandles := inputCount + outputCount

	c.logger.WithFields(logrus.Fields{
		"queue_name":     queueName,
		"input_handles":  inputCount,
		"output_handles": outputCount,
		"total_handles":  totalHandles,
	}).Debug("Retrieved queue handles via MQINQ")

	// For each handle count, create a representative HandleInfo record
	// In a real implementation with direct monmqi access, you would get
	// detailed per-handle information including PID, user, connection name
	for i := int32(0); i < inputCount; i++ {
		handles = append(handles, &HandleInfo{
			QueueName:    queueName,
			HandleState:  "Open",
			OpenMode:     "Input",
			CreationTime: time.Now(),
			LastUsedTime: time.Now(),
		})
	}

	for i := int32(0); i < outputCount; i++ {
		handles = append(handles, &HandleInfo{
			QueueName:    queueName,
			HandleState:  "Open",
			OpenMode:     "Output",
			CreationTime: time.Now(),
			LastUsedTime: time.Now(),
		})
	}

	return handles, nil
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
