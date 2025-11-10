package mqclient

import (
	"fmt"
	"time"

	"github.com/atulksin/ibmmq-go-stat-otel/pkg/config"
	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
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
