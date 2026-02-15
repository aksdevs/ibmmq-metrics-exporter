#include "ibmmq_exporter/mqclient.h"
#include "ibmmq_exporter/pcf_inquiry.h"

#include <algorithm>
#include <cstring>
#include <stdexcept>
#include <thread>

#include <spdlog/spdlog.h>

namespace ibmmq_exporter {

// Helper to check if a queue name matches an HLQ pattern
static bool matches_hlq_pattern(const std::string& queue_name, const std::string& pattern) {
    if (pattern == "*") return true;

    // Pattern ending with .*
    if (pattern.size() > 2 && pattern.substr(pattern.size() - 2) == ".*") {
        auto prefix = pattern.substr(0, pattern.size() - 2);
        return queue_name.size() > prefix.size() &&
               queue_name.substr(0, prefix.size() + 1) == prefix + ".";
    }

    // Pattern ending with .**
    if (pattern.size() > 3 && pattern.substr(pattern.size() - 3) == ".**") {
        auto prefix = pattern.substr(0, pattern.size() - 3);
        return queue_name.size() > prefix.size() &&
               queue_name.substr(0, prefix.size() + 1) == prefix + ".";
    }

    return queue_name == pattern;
}

bool queue_matches_exclusion(const std::string& queue_name,
                             const std::vector<std::string>& patterns) {
    return std::any_of(patterns.begin(), patterns.end(),
                       [&](const auto& p) { return matches_hlq_pattern(queue_name, p); });
}

MQClient::MQClient(const MQConfig& config) : config_(config) {}

MQClient::~MQClient() {
    if (connected_) {
        try { disconnect(); } catch (...) {}
    }
}

void MQClient::connect() {
    if (connected_) return;

    bool client_mode = config_.is_client_mode();
    spdlog::info("Connecting to IBM MQ: QM={}, Channel={}, Conn={}, Mode={}",
                 config_.queue_manager, config_.channel, config_.get_connection_name(),
                 client_mode ? "client" : "binding");

    MQCNO cno = MQCNO_DEFAULT;
    MQCD cd = MQCD_CLIENT_CONN_DEFAULT;
    MQCSP csp = MQCSP_DEFAULT;

    if (client_mode) {
        cno.Options = MQCNO_CLIENT_BINDING;
        std::strncpy(cd.ChannelName, config_.channel.c_str(), sizeof(cd.ChannelName) - 1);
        std::strncpy(cd.ConnectionName, config_.get_connection_name().c_str(), sizeof(cd.ConnectionName) - 1);

        if (!config_.cipher_spec.empty()) {
            std::strncpy(cd.SSLCipherSpec, config_.cipher_spec.c_str(), sizeof(cd.SSLCipherSpec) - 1);
        }

        cno.ClientConnPtr = &cd;
    } else {
        // Binding mode - connect locally
        cno.Options = MQCNO_LOCAL_BINDING;
    }

    auto usr = config_.get_user();
    if (!usr.empty()) {
        csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
        csp.CSPUserIdPtr   = const_cast<char*>(usr.c_str());
        csp.CSPUserIdLength = static_cast<MQLONG>(usr.size());
        csp.CSPPasswordPtr  = const_cast<char*>(config_.password.c_str());
        csp.CSPPasswordLength = static_cast<MQLONG>(config_.password.size());
        cno.SecurityParmsPtr = &csp;
        cno.Version = MQCNO_VERSION_5;
    }

    MQLONG cc = 0, rc = 0;
    MQCONNX(config_.queue_manager.c_str(), &cno, &hconn_, &cc, &rc);

    if (cc == MQCC_FAILED) {
        throw std::runtime_error("Failed to connect to queue manager " +
                                 config_.queue_manager + " (RC=" + std::to_string(rc) + ")");
    }

    connected_ = true;
    spdlog::info("Successfully connected to IBM MQ");

    // Detect remote platform
    detect_platform();
}

void MQClient::disconnect() {
    if (!connected_) return;

    spdlog::info("Disconnecting from IBM MQ");

    unsubscribe_all();

    if (stats_open_) close_queue(stats_queue_);
    if (acct_open_)  close_queue(acct_queue_);

    MQLONG cc = 0, rc = 0;
    MQDISC(&hconn_, &cc, &rc);

    if (cc == MQCC_FAILED) {
        spdlog::error("Error disconnecting from queue manager (RC={})", rc);
    }

    connected_ = false;
    stats_open_ = false;
    acct_open_ = false;
    spdlog::info("Disconnected from IBM MQ");
}

void MQClient::detect_platform() {
    // Open QM object for inquiry
    MQOD od = MQOD_DEFAULT;
    od.ObjectType = MQOT_Q_MGR;

    MQHOBJ hobj = 0;
    MQLONG cc = 0, rc = 0;
    MQOPEN(hconn_, &od, MQOO_INQUIRE, &hobj, &cc, &rc);

    if (cc == MQCC_FAILED) {
        spdlog::debug("Cannot open QM for platform inquiry (RC={})", rc);
        platform_ = platform::local_platform();
        return;
    }

    MQLONG selectors[] = { MQIA_PLATFORM };
    MQLONG attrs[1] = {0};
    MQINQ(hconn_, hobj, 1, selectors, 1, attrs, 0, nullptr, &cc, &rc);
    MQCLOSE(hconn_, &hobj, MQCO_NONE, &cc, &rc);

    if (attrs[0] > 0) {
        platform_ = attrs[0];
    } else {
        platform_ = platform::local_platform();
    }
    spdlog::info("Detected QM platform: {} ({})", platform::platform_name(platform_), platform_);
}

std::string MQClient::get_platform_string() const {
    return platform::platform_name(platform_);
}

MQHOBJ MQClient::open_queue(const std::string& queue_name, MQLONG options) {
    MQOD od = MQOD_DEFAULT;
    od.ObjectType = MQOT_Q;
    std::strncpy(od.ObjectName, queue_name.c_str(), sizeof(od.ObjectName) - 1);

    MQHOBJ hobj = 0;
    MQLONG cc = 0, rc = 0;
    MQOPEN(hconn_, &od, options, &hobj, &cc, &rc);

    if (cc == MQCC_FAILED) {
        throw std::runtime_error("Failed to open queue " + queue_name +
                                 " (RC=" + std::to_string(rc) + ")");
    }
    return hobj;
}

MQHOBJ MQClient::open_queue(const std::string& queue_name, MQLONG options,
                            const std::string& dynamic_q_name, std::string& resolved_name) {
    MQOD od = MQOD_DEFAULT;
    od.ObjectType = MQOT_Q;
    std::strncpy(od.ObjectName, queue_name.c_str(), sizeof(od.ObjectName) - 1);
    std::strncpy(od.DynamicQName, dynamic_q_name.c_str(), sizeof(od.DynamicQName) - 1);

    MQHOBJ hobj = 0;
    MQLONG cc = 0, rc = 0;
    MQOPEN(hconn_, &od, options, &hobj, &cc, &rc);

    if (cc == MQCC_FAILED) {
        throw std::runtime_error("Failed to open dynamic queue from " + queue_name +
                                 " (RC=" + std::to_string(rc) + ")");
    }

    resolved_name = std::string(od.ObjectName, sizeof(od.ObjectName));
    auto pos = resolved_name.find_last_not_of(" \0");
    if (pos != std::string::npos) resolved_name.resize(pos + 1);

    return hobj;
}

void MQClient::close_queue(MQHOBJ& hobj) {
    if (hobj == 0) return;
    MQLONG cc = 0, rc = 0;
    MQCLOSE(hconn_, &hobj, MQCO_NONE, &cc, &rc);
    hobj = 0;
}

void MQClient::open_stats_queue(const std::string& queue_name) {
    if (!connected_) throw std::runtime_error("Not connected to queue manager");
    stats_queue_ = open_queue(queue_name,
                              MQOO_INPUT_AS_Q_DEF | MQOO_FAIL_IF_QUIESCING);
    stats_open_ = true;
    spdlog::info("Opened statistics queue: {}", queue_name);
}

void MQClient::open_accounting_queue(const std::string& queue_name) {
    if (!connected_) throw std::runtime_error("Not connected to queue manager");
    acct_queue_ = open_queue(queue_name,
                             MQOO_INPUT_AS_Q_DEF | MQOO_FAIL_IF_QUIESCING);
    acct_open_ = true;
    spdlog::info("Opened accounting queue: {}", queue_name);
}

std::optional<MQMessage> MQClient::get_message(const std::string& queue_type) {
    MQHOBJ hobj = 0;
    if (queue_type == "stats") {
        if (!stats_open_) return std::nullopt;
        hobj = stats_queue_;
    } else if (queue_type == "accounting") {
        if (!acct_open_) return std::nullopt;
        hobj = acct_queue_;
    } else {
        throw std::runtime_error("Unknown queue type: " + queue_type);
    }

    MQMD md = MQMD_DEFAULT;
    MQGMO gmo = MQGMO_DEFAULT;
    gmo.Options = MQGMO_NO_WAIT | MQGMO_FAIL_IF_QUIESCING | MQGMO_CONVERT;
    gmo.WaitInterval = 1000;

    constexpr size_t BUF_SIZE = 100 * 1024; // 100KB
    std::vector<uint8_t> buffer(BUF_SIZE);

    MQLONG datalen = 0, cc = 0, rc = 0;
    MQGET(hconn_, hobj, &md, &gmo,
          static_cast<MQLONG>(buffer.size()),
          buffer.data(), &datalen, &cc, &rc);

    if (cc == MQCC_FAILED) {
        if (rc == MQRC_NO_MSG_AVAILABLE)
            return std::nullopt;
        if (rc == MQRC_CONNECTION_BROKEN) {
            connected_ = false;
            throw std::runtime_error("Connection broken (RC=2009)");
        }
        throw std::runtime_error("Failed to get message from " + queue_type +
                                 " queue (RC=" + std::to_string(rc) + ")");
    }

    MQMessage msg;
    msg.data.assign(buffer.begin(), buffer.begin() + datalen);
    msg.type = queue_type;
    msg.msg_type = md.MsgType;
    msg.put_date = std::string(md.PutDate, sizeof(md.PutDate));
    msg.put_time = std::string(md.PutTime, sizeof(md.PutTime));
    msg.format = std::string(md.Format, sizeof(md.Format));
    msg.msg_id.assign(md.MsgId, md.MsgId + sizeof(md.MsgId));

    spdlog::debug("Retrieved message from {} queue, size={}", queue_type, datalen);
    return msg;
}

std::vector<MQMessage> MQClient::get_all_messages(const std::string& queue_type) {
    std::vector<MQMessage> messages;

    while (true) {
        auto msg = get_message(queue_type);
        if (!msg) break;
        messages.push_back(std::move(*msg));
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }

    spdlog::info("Retrieved {} messages from {} queue", messages.size(), queue_type);
    return messages;
}

std::optional<QueueStats> MQClient::get_queue_stats(const std::string& queue_name) {
    if (!connected_) return std::nullopt;

    MQHOBJ hobj = 0;
    try {
        hobj = open_queue(queue_name, MQOO_INQUIRE);
    } catch (...) {
        spdlog::debug("Failed to open queue {} for inquiry", queue_name);
        return std::nullopt;
    }

    MQLONG selectors[] = { MQIA_CURRENT_Q_DEPTH, MQIA_OPEN_INPUT_COUNT, MQIA_OPEN_OUTPUT_COUNT };
    MQLONG attrs[3] = {0};
    MQLONG cc = 0, rc = 0;

    MQINQ(hconn_, hobj, 3, selectors, 3, attrs, 0, nullptr, &cc, &rc);
    close_queue(hobj);

    if (cc == MQCC_FAILED) {
        spdlog::debug("Failed to inquire queue {} (RC={})", queue_name, rc);
        return std::nullopt;
    }

    QueueStats stats;
    stats.queue_name = queue_name;
    stats.current_depth = attrs[0];
    stats.open_input_count = attrs[1];
    stats.open_output_count = attrs[2];

    spdlog::debug("Queue {} stats: depth={}, input={}, output={}",
                  queue_name, stats.current_depth, stats.open_input_count, stats.open_output_count);
    return stats;
}

std::optional<QueueInfo> MQClient::get_queue_info(const std::string& queue_name) {
    if (!connected_) return std::nullopt;

    MQHOBJ hobj = 0;
    try {
        hobj = open_queue(queue_name, MQOO_INQUIRE);
    } catch (...) {
        return std::nullopt;
    }

    MQLONG selectors[] = { MQIA_CURRENT_Q_DEPTH, MQIA_OPEN_INPUT_COUNT,
                           MQIA_OPEN_OUTPUT_COUNT, MQIA_MAX_Q_DEPTH };
    MQLONG attrs[4] = {0};
    MQLONG cc = 0, rc = 0;

    MQINQ(hconn_, hobj, 4, selectors, 4, attrs, 0, nullptr, &cc, &rc);
    close_queue(hobj);

    if (cc == MQCC_FAILED) return std::nullopt;

    QueueInfo info;
    info.queue_name = queue_name;
    info.current_depth = attrs[0];
    info.open_input_count = attrs[1];
    info.open_output_count = attrs[2];
    info.max_queue_depth = attrs[3];
    return info;
}

std::vector<HandleInfo> MQClient::get_queue_handles(const std::string& queue_name) {
    if (!connected_) return {};

    MQHOBJ hobj = 0;
    try {
        hobj = open_queue(queue_name, MQOO_INQUIRE);
    } catch (...) {
        return {};
    }

    MQLONG selectors[] = { MQIA_OPEN_INPUT_COUNT, MQIA_OPEN_OUTPUT_COUNT };
    MQLONG attrs[2] = {0};
    MQLONG cc = 0, rc = 0;

    MQINQ(hconn_, hobj, 2, selectors, 2, attrs, 0, nullptr, &cc, &rc);
    close_queue(hobj);

    spdlog::debug("Queue {} handles: input={}, output={}", queue_name, attrs[0], attrs[1]);
    return {};
}

// --- Reusable PCF command helper ---

std::vector<std::vector<uint8_t>> MQClient::send_pcf_command(const std::vector<uint8_t>& cmd) {
    std::vector<std::vector<uint8_t>> responses;
    if (!connected_) return responses;

    // Create dynamic reply queue
    std::string reply_q_name;
    MQHOBJ reply_hobj = 0;
    try {
        reply_hobj = open_queue("SYSTEM.DEFAULT.MODEL.QUEUE",
                                MQOO_INPUT_EXCLUSIVE, "AMQ.*", reply_q_name);
    } catch (...) {
        spdlog::debug("Failed to create dynamic reply queue for PCF");
        return responses;
    }

    if (reply_q_name.empty() || reply_q_name == "SYSTEM.DEFAULT.MODEL.QUEUE") {
        close_queue(reply_hobj);
        return responses;
    }

    // Open command queue and send
    MQHOBJ cmd_hobj = 0;
    try {
        cmd_hobj = open_queue("SYSTEM.ADMIN.COMMAND.QUEUE", MQOO_OUTPUT);
    } catch (...) {
        close_queue(reply_hobj);
        return responses;
    }

    MQMD md = MQMD_DEFAULT;
    std::memcpy(md.Format, "MQADMIN ", sizeof(md.Format));
    std::strncpy(md.ReplyToQ, reply_q_name.c_str(), sizeof(md.ReplyToQ) - 1);
    md.MsgType = MQMT_REQUEST;

    for (size_t i = 0; i < sizeof(md.CorrelId); ++i)
        md.CorrelId[i] = static_cast<uint8_t>(65 + (i % 26));

    MQPMO pmo = MQPMO_DEFAULT;
    pmo.Options = MQPMO_NONE;

    MQLONG cc = 0, rc = 0;
    MQPUT(hconn_, cmd_hobj, &md, &pmo,
          static_cast<MQLONG>(cmd.size()),
          const_cast<uint8_t*>(cmd.data()), &cc, &rc);
    close_queue(cmd_hobj);

    if (cc == MQCC_FAILED) {
        spdlog::debug("Failed to send PCF command (RC={})", rc);
        close_queue(reply_hobj);
        return responses;
    }

    // Read all response messages (loop until MQCFC_LAST or no more messages)
    constexpr size_t BUF_SIZE = 64 * 1024;
    std::vector<uint8_t> resp_buf(BUF_SIZE);

    for (int i = 0; i < 1000; ++i) { // safety limit
        MQMD resp_md = MQMD_DEFAULT;
        MQGMO gmo = MQGMO_DEFAULT;
        gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
        gmo.WaitInterval = 5000;

        MQLONG datalen = 0;
        MQGET(hconn_, reply_hobj, &resp_md, &gmo,
              static_cast<MQLONG>(resp_buf.size()),
              resp_buf.data(), &datalen, &cc, &rc);

        if (cc == MQCC_FAILED) break;

        responses.emplace_back(resp_buf.begin(), resp_buf.begin() + datalen);

        // Check if this is the last response (Control == MQCFC_LAST)
        // Control field is at offset 20 in MQCFH (native byte order after MQGMO_CONVERT)
        if (datalen >= 24) {
            uint32_t control = 0;
            std::memcpy(&control, &resp_buf[20], sizeof(control));
            if (control == 1) break; // MQCFC_LAST
        }
    }

    close_queue(reply_hobj);
    spdlog::debug("Received {} PCF response(s)", responses.size());
    return responses;
}

std::vector<HandleInfo> MQClient::get_queue_handle_details_by_pcf(const std::string& queue_name) {
    spdlog::info("Retrieving handle details via PCF for queue {}", queue_name);

    auto cmd = PCFInquiry::build_inquire_q_cmd(queue_name);
    auto responses = send_pcf_command(cmd);

    if (responses.empty()) return {};

    // Parse first response
    auto details = PCFInquiry::parse_queue_status_response(
        responses[0].data(), responses[0].size());

    std::vector<HandleInfo> handles;
    handles.reserve(details.size());
    for (const auto& d : details) {
        HandleInfo h;
        h.queue_name = d.queue_name;
        h.application_tag = d.application_tag;
        h.channel_name = d.channel_name;
        h.connection_name = d.connection_name;
        h.user_identifier = d.user_id;
        h.process_id = d.process_id;
        handles.push_back(std::move(h));
    }

    spdlog::info("Found {} handles for queue {}", handles.size(), queue_name);
    return handles;
}

// --- PCF status inquiry methods ---

std::vector<ChannelStatusDetails> MQClient::get_channel_status(const std::string& pattern) {
    auto cmd = PCFInquiry::build_inquire_channel_status_cmd(pattern);
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_channel_status_response(responses);
}

std::vector<TopicStatusDetails> MQClient::get_topic_status(const std::string& pattern) {
    auto cmd = PCFInquiry::build_inquire_topic_status_cmd(pattern);
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_topic_status_response(responses);
}

std::vector<SubStatusDetails> MQClient::get_sub_status(const std::string& pattern) {
    auto cmd = PCFInquiry::build_inquire_sub_status_cmd(pattern);
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_sub_status_response(responses);
}

std::optional<QMgrStatusDetails> MQClient::get_qmgr_status() {
    auto cmd = PCFInquiry::build_inquire_qmgr_status_cmd();
    auto responses = send_pcf_command(cmd);
    auto results = PCFInquiry::parse_qmgr_status_response(responses);
    if (results.empty()) return std::nullopt;
    return results[0];
}

std::vector<ClusterQMgrDetails> MQClient::get_cluster_status() {
    auto cmd = PCFInquiry::build_inquire_cluster_qmgr_cmd();
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_cluster_qmgr_response(responses);
}

std::vector<UsageBPDetails> MQClient::get_usage_bp_status() {
    auto cmd = PCFInquiry::build_inquire_usage_cmd(1); // BP
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_usage_bp_response(responses);
}

std::vector<UsagePSDetails> MQClient::get_usage_ps_status() {
    auto cmd = PCFInquiry::build_inquire_usage_cmd(2); // PS
    auto responses = send_pcf_command(cmd);
    return PCFInquiry::parse_usage_ps_response(responses);
}

bool MQClient::reset_queue_stats(const std::string& queue_name) {
    auto cmd = PCFInquiry::build_reset_q_stats_cmd(queue_name);
    auto responses = send_pcf_command(cmd);
    return !responses.empty();
}

std::vector<std::string> MQClient::discover_queues(const std::string& pattern) {
    auto cmd = PCFInquiry::build_inquire_q_cmd(pattern);
    auto responses = send_pcf_command(cmd);

    std::vector<std::string> queues;
    for (const auto& resp : responses) {
        auto details = PCFInquiry::parse_queue_status_response(resp.data(), resp.size());
        for (const auto& d : details) {
            if (!d.queue_name.empty())
                queues.push_back(d.queue_name);
        }
    }
    spdlog::info("Discovered {} queues matching '{}'", queues.size(), pattern);
    return queues;
}

// --- Topic subscription ---

void MQClient::subscribe_to_topic(const std::string& topic_string) {
    if (!connected_) return;

    MQSD sd = MQSD_DEFAULT;
    sd.Options = MQSO_CREATE | MQSO_NON_DURABLE | MQSO_MANAGED | MQSO_FAIL_IF_QUIESCING;

    // Set the topic string
    std::string topic_copy = topic_string;
    sd.ObjectString.VSPtr = topic_copy.data();
    sd.ObjectString.VSLength = static_cast<MQLONG>(topic_copy.size());

    MQHOBJ hobj = 0;
    MQHSUB hsub = 0;
    MQLONG cc = 0, rc = 0;

    MQSUB(hconn_, &sd, &hobj, &hsub, &cc, &rc);

    if (cc == MQCC_FAILED) {
        spdlog::warn("Failed to subscribe to topic '{}' (RC={})", topic_string, rc);
        return;
    }

    subscriptions_.push_back({hobj, hsub});
    spdlog::info("Subscribed to topic: {}", topic_string);
}

std::vector<MQMessage> MQClient::receive_publications() {
    std::vector<MQMessage> messages;
    if (!connected_) return messages;

    constexpr size_t BUF_SIZE = 64 * 1024;
    std::vector<uint8_t> buffer(BUF_SIZE);

    for (auto& sub : subscriptions_) {
        MQMD md = MQMD_DEFAULT;
        MQGMO gmo = MQGMO_DEFAULT;
        gmo.Options = MQGMO_NO_WAIT | MQGMO_CONVERT;

        MQLONG datalen = 0, cc = 0, rc = 0;
        MQGET(hconn_, sub.hobj, &md, &gmo,
              static_cast<MQLONG>(buffer.size()),
              buffer.data(), &datalen, &cc, &rc);

        if (cc == MQCC_OK && datalen > 0) {
            MQMessage msg;
            msg.data.assign(buffer.begin(), buffer.begin() + datalen);
            msg.type = "publication";
            msg.msg_type = md.MsgType;
            msg.format = std::string(md.Format, sizeof(md.Format));
            messages.push_back(std::move(msg));
        }
    }
    return messages;
}

void MQClient::unsubscribe_all() {
    for (auto& sub : subscriptions_) {
        MQLONG cc = 0, rc = 0;
        if (sub.hsub != 0) {
            MQCLOSE(hconn_, reinterpret_cast<MQHOBJ*>(&sub.hsub), MQCO_NONE, &cc, &rc);
        }
        if (sub.hobj != 0) {
            MQCLOSE(hconn_, &sub.hobj, MQCO_NONE, &cc, &rc);
        }
    }
    subscriptions_.clear();
}

} // namespace ibmmq_exporter
