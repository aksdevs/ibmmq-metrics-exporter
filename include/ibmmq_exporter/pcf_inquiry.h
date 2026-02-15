#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace ibmmq_exporter {

struct QueueHandleDetails {
    std::string queue_name;
    std::string application_tag;
    std::string channel_name;
    std::string connection_name;
    std::string user_id;
    int32_t     process_id{0};
    std::string input_mode;
    std::string output_mode;
};

struct ChannelStatusDetails {
    std::string channel_name;
    std::string connection_name;
    std::string remote_qmgr;
    std::string job_name;
    std::string ssl_cipher;
    int32_t     channel_type{0};
    int32_t     status{0};
    int32_t     msgs{0};
    int64_t     bytes_sent{0};
    int64_t     bytes_received{0};
    int32_t     batches{0};
    int32_t     substate{0};
    int32_t     instance_type{0};
};

struct TopicStatusDetails {
    std::string topic_string;
    std::string topic_name;
    int32_t     topic_type{0};
    int32_t     pub_count{0};
    int32_t     sub_count{0};
};

struct SubStatusDetails {
    std::string sub_name;
    std::string sub_id;
    std::string topic_string;
    std::string destination;
    int32_t     sub_type{0};
    int32_t     durable{0};
};

struct QMgrStatusDetails {
    std::string qmgr_name;
    std::string description;
    int32_t     status{0};
    int32_t     chinit_status{0};
    int32_t     connection_count{0};
    int32_t     cmd_server_status{0};
    std::string start_date;
    std::string start_time;
};

struct ClusterQMgrDetails {
    std::string cluster_name;
    std::string qmgr_name;
    int32_t     qm_type{0};
    int32_t     status{0};
};

struct UsageBPDetails {
    int32_t buffer_pool{0};
    int32_t free_buffers{0};
    int32_t total_buffers{0};
    int32_t location{0};
    int32_t page_class{0};
};

struct UsagePSDetails {
    int32_t pageset_id{0};
    int32_t buffer_pool{0};
    int32_t total_pages{0};
    int32_t unused_pages{0};
    int32_t persist_pages{0};
    int32_t nonpersist_pages{0};
    int32_t restart_pages{0};
    int32_t expand_count{0};
};

class PCFInquiry {
public:
    // Build a PCF INQUIRE_Q command (MQCMD_INQUIRE_Q = 3)
    static std::vector<uint8_t> build_inquire_q_cmd(const std::string& queue_name);

    // Build a PCF INQUIRE_Q_STATUS command (MQCMD_INQUIRE_Q_STATUS = 34)
    static std::vector<uint8_t> build_inquire_q_status_cmd(const std::string& queue_name);

    // Build PCF INQUIRE_CHANNEL_STATUS command
    static std::vector<uint8_t> build_inquire_channel_status_cmd(const std::string& channel_pattern);

    // Build PCF INQUIRE_TOPIC_STATUS command
    static std::vector<uint8_t> build_inquire_topic_status_cmd(const std::string& topic_pattern);

    // Build PCF INQUIRE_SUB_STATUS command
    static std::vector<uint8_t> build_inquire_sub_status_cmd(const std::string& sub_pattern);

    // Build PCF INQUIRE_Q_MGR_STATUS command
    static std::vector<uint8_t> build_inquire_qmgr_status_cmd();

    // Build PCF INQUIRE_CLUSTER_Q_MGR command
    static std::vector<uint8_t> build_inquire_cluster_qmgr_cmd();

    // Build PCF INQUIRE_USAGE command (usage_type: 1=BP, 2=PS)
    static std::vector<uint8_t> build_inquire_usage_cmd(int32_t usage_type);

    // Build PCF RESET_Q_STATS command
    static std::vector<uint8_t> build_reset_q_stats_cmd(const std::string& queue_name);

    // Parse responses
    static std::vector<QueueHandleDetails> parse_queue_status_response(
        const uint8_t* data, size_t len);

    static std::vector<ChannelStatusDetails> parse_channel_status_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<TopicStatusDetails> parse_topic_status_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<SubStatusDetails> parse_sub_status_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<QMgrStatusDetails> parse_qmgr_status_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<ClusterQMgrDetails> parse_cluster_qmgr_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<UsageBPDetails> parse_usage_bp_response(
        const std::vector<std::vector<uint8_t>>& responses);

    static std::vector<UsagePSDetails> parse_usage_ps_response(
        const std::vector<std::vector<uint8_t>>& responses);

private:
    static void append_int32(std::vector<uint8_t>& buf, int32_t value);

    // Build a generic PCF header + optional string/int params
    static std::vector<uint8_t> build_pcf_cmd(int32_t command, int32_t param_count);
    static void append_string_param(std::vector<uint8_t>& buf, int32_t param_id, const std::string& value);
    static void append_integer_param(std::vector<uint8_t>& buf, int32_t param_id, int32_t value);

    // Generic response parser helpers
    static std::string read_string_param(const uint8_t* data, size_t len);
    static int32_t read_int_param(const uint8_t* data, size_t len);

    static void parse_string_param(const uint8_t* param_data, size_t len,
                                   QueueHandleDetails& handle);
    static void parse_integer_param(const uint8_t* param_data, size_t len,
                                    QueueHandleDetails& handle);
};

} // namespace ibmmq_exporter
