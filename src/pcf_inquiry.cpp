#include "ibmmq_exporter/pcf_inquiry.h"

#include <algorithm>
#include <cstring>

#include <spdlog/spdlog.h>

namespace ibmmq_exporter {

// --- Low-level helpers ---
// All integer read/write uses native (platform) byte order.
// MQ handles encoding conversion via MQMD.Encoding and MQGMO_CONVERT.

void PCFInquiry::append_int32(std::vector<uint8_t>& buf, int32_t value) {
    auto p = reinterpret_cast<const uint8_t*>(&value);
    buf.insert(buf.end(), p, p + 4);
}

static uint32_t read_int32(const uint8_t* p) {
    uint32_t val;
    std::memcpy(&val, p, 4);
    return val;
}

static int64_t read_int64(const uint8_t* p) {
    int64_t val;
    std::memcpy(&val, p, 8);
    return val;
}

static std::string trim_mq_string(const std::string& s) {
    auto pos = s.find_last_not_of(std::string("\0 ", 2));
    if (pos != std::string::npos) return s.substr(0, pos + 1);
    return {};
}

// --- Generic PCF builders ---

// MQCFH layout (36 bytes, 9 x MQLONG):
//   offset  0: Type
//   offset  4: StrucLength
//   offset  8: Version
//   offset 12: Command
//   offset 16: MsgSeqNumber
//   offset 20: Control
//   offset 24: CompCode
//   offset 28: Reason
//   offset 32: ParameterCount
constexpr size_t MQCFH_SIZE = 36;

std::vector<uint8_t> PCFInquiry::build_pcf_cmd(int32_t command, int32_t param_count) {
    std::vector<uint8_t> buf;
    buf.reserve(512);
    append_int32(buf, 1);              // Type = MQCFT_COMMAND
    append_int32(buf, MQCFH_SIZE);     // StrucLength = 36 bytes
    append_int32(buf, 1);              // Version = 1
    append_int32(buf, command);
    append_int32(buf, 1);              // MsgSeqNumber
    append_int32(buf, 1);              // Control = MQCFC_LAST
    append_int32(buf, 0);              // CompCode
    append_int32(buf, 0);              // Reason
    append_int32(buf, param_count);    // ParameterCount
    return buf;
}

// MQCFST layout:
//   offset  0: Type (4) = MQCFT_STRING
//   offset  4: StrucLength (4)
//   offset  8: Parameter (4)
//   offset 12: CodedCharSetId (4)
//   offset 16: StringLength (4)
//   offset 20: String[N] (padded to 4-byte boundary)
void PCFInquiry::append_string_param(std::vector<uint8_t>& buf, int32_t param_id, const std::string& value) {
    size_t param_start = buf.size();
    append_int32(buf, 4);          // Type = MQCFT_STRING
    size_t len_pos = buf.size();
    append_int32(buf, 0);          // placeholder for StrucLength
    append_int32(buf, param_id);   // Parameter
    append_int32(buf, 0);          // CodedCharSetId = MQCCSI_DEFAULT

    auto str_len = static_cast<int32_t>(value.size());
    append_int32(buf, str_len);    // StringLength
    buf.insert(buf.end(), value.begin(), value.end());

    int padding = (4 - (str_len % 4)) % 4;
    for (int i = 0; i < padding; ++i) buf.push_back(0);

    // Patch StrucLength in native byte order
    auto param_len = static_cast<int32_t>(buf.size() - param_start);
    std::memcpy(&buf[len_pos], &param_len, 4);
}

// MQCFIN layout:
//   offset  0: Type (4) = MQCFT_INTEGER
//   offset  4: StrucLength (4) = 16
//   offset  8: Parameter (4)
//   offset 12: Value (4)
void PCFInquiry::append_integer_param(std::vector<uint8_t>& buf, int32_t param_id, int32_t value) {
    append_int32(buf, 3);          // Type = MQCFT_INTEGER
    append_int32(buf, 16);         // StrucLength
    append_int32(buf, param_id);   // Parameter
    append_int32(buf, value);      // Value
}

// --- Command builders ---

std::vector<uint8_t> PCFInquiry::build_inquire_q_cmd(const std::string& queue_name) {
    auto buf = build_pcf_cmd(3, 1); // MQCMD_INQUIRE_Q, 1 param
    append_string_param(buf, 2016, queue_name); // MQCA_Q_NAME
    spdlog::debug("Built INQUIRE_Q PCF command for {}, size={}", queue_name, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_q_status_cmd(const std::string& queue_name) {
    auto buf = build_pcf_cmd(34, 2); // MQCMD_INQUIRE_Q_STATUS, 2 params
    append_string_param(buf, 2016, queue_name); // MQCA_Q_NAME
    append_integer_param(buf, 1238, 2); // MQIACF_Q_STATUS_TYPE = HANDLE
    spdlog::debug("Built INQUIRE_Q_STATUS PCF command for {}, size={}", queue_name, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_channel_status_cmd(const std::string& channel_pattern) {
    auto buf = build_pcf_cmd(41, 1); // MQCMD_INQUIRE_CHANNEL_STATUS
    append_string_param(buf, 3501, channel_pattern); // MQCACH_CHANNEL_NAME
    spdlog::debug("Built INQUIRE_CHANNEL_STATUS for {}, size={}", channel_pattern, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_topic_status_cmd(const std::string& topic_pattern) {
    auto buf = build_pcf_cmd(87, 1); // MQCMD_INQUIRE_TOPIC_STATUS
    append_string_param(buf, 2094, topic_pattern); // MQCA_TOPIC_STRING
    spdlog::debug("Built INQUIRE_TOPIC_STATUS for {}, size={}", topic_pattern, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_sub_status_cmd(const std::string& sub_pattern) {
    auto buf = build_pcf_cmd(92, 1); // MQCMD_INQUIRE_SUB_STATUS
    append_string_param(buf, 2095, sub_pattern); // MQCA_SUB_NAME
    spdlog::debug("Built INQUIRE_SUB_STATUS for {}, size={}", sub_pattern, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_qmgr_status_cmd() {
    auto buf = build_pcf_cmd(161, 0); // MQCMD_INQUIRE_Q_MGR_STATUS, 0 params
    spdlog::debug("Built INQUIRE_Q_MGR_STATUS, size={}", buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_cluster_qmgr_cmd() {
    auto buf = build_pcf_cmd(71, 1); // MQCMD_INQUIRE_CLUSTER_Q_MGR
    append_string_param(buf, 2004, "*"); // MQCA_CLUSTER_NAME = wildcard
    spdlog::debug("Built INQUIRE_CLUSTER_Q_MGR, size={}", buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_inquire_usage_cmd(int32_t usage_type) {
    auto buf = build_pcf_cmd(84, 1); // MQCMD_INQUIRE_USAGE
    append_integer_param(buf, 1125, usage_type); // MQIACF_USAGE_TYPE
    spdlog::debug("Built INQUIRE_USAGE type={}, size={}", usage_type, buf.size());
    return buf;
}

std::vector<uint8_t> PCFInquiry::build_reset_q_stats_cmd(const std::string& queue_name) {
    auto buf = build_pcf_cmd(17, 1); // MQCMD_RESET_Q_STATS
    append_string_param(buf, 2016, queue_name); // MQCA_Q_NAME
    spdlog::debug("Built RESET_Q_STATS for {}, size={}", queue_name, buf.size());
    return buf;
}

// --- Generic response parsing helpers ---

std::string PCFInquiry::read_string_param(const uint8_t* data, size_t len) {
    // data points past Type+StrucLength (i.e., at Parameter field)
    // Layout: Parameter(4) + CodedCharSetId(4) + StringLength(4) + String[N]
    if (len < 12) return {};
    uint32_t str_len = read_int32(data + 8);
    if (12 + str_len > len) return {};
    return trim_mq_string(std::string(reinterpret_cast<const char*>(data + 12), str_len));
}

int32_t PCFInquiry::read_int_param(const uint8_t* data, size_t len) {
    // data points past Type+StrucLength (i.e., at Parameter field)
    // Layout: Parameter(4) + Value(4)
    if (len < 8) return 0;
    return static_cast<int32_t>(read_int32(data + 4));
}

// Parse a single PCF response message, calling a visitor lambda per parameter.
// The visitor receives: (param_type, param_id, pointer_to_param_start, param_struct_length)
template <typename Visitor>
static void parse_pcf_response_params(const uint8_t* data, size_t len, Visitor&& visitor) {
    if (len < MQCFH_SIZE) return;

    // CompCode at offset 24, Reason at offset 28
    uint32_t comp_code = read_int32(data + 24);
    if (comp_code != 0) {
        spdlog::debug("PCF response error: comp_code={}, reason={}", comp_code, read_int32(data + 28));
        return;
    }

    size_t offset = MQCFH_SIZE; // parameters start after 36-byte header
    while (offset + 8 <= len) {
        uint32_t param_type = read_int32(data + offset);       // Type
        uint32_t param_len  = read_int32(data + offset + 4);   // StrucLength
        if (param_len == 0 || offset + param_len > len) break;

        uint32_t param_id = 0;
        if (offset + 12 <= len) param_id = read_int32(data + offset + 8); // Parameter

        visitor(param_type, param_id, data + offset, param_len);
        offset += param_len;
    }
}

// Helper: read a string value from a MQCFST parameter structure.
// pdata points to the start of the parameter (Type field).
// MQCFST: Type(4) StrucLength(4) Parameter(4) CodedCharSetId(4) StringLength(4) String[N]
//         0       4              8             12                16              20
static std::string read_pcf_string(const uint8_t* pdata, uint32_t plen) {
    if (plen < 24) return {};
    uint32_t str_len = read_int32(pdata + 16);
    if (20 + str_len > plen) str_len = plen - 20;
    return trim_mq_string(std::string(
        reinterpret_cast<const char*>(pdata + 20), str_len));
}

// Helper: read an integer value from a MQCFIN parameter structure.
// MQCFIN: Type(4) StrucLength(4) Parameter(4) Value(4)
//         0       4              8             12
static int32_t read_pcf_int32(const uint8_t* pdata, uint32_t plen) {
    if (plen < 16) return 0;
    return static_cast<int32_t>(read_int32(pdata + 12));
}

// Helper: read an int64 value from a MQCFIN64 parameter structure.
// MQCFIN64: Type(4) StrucLength(4) Parameter(4) Reserved(4) Value(8)
//           0       4              8             12          16
static int64_t read_pcf_int64(const uint8_t* pdata, uint32_t plen) {
    if (plen < 24) return 0;
    return read_int64(pdata + 16);
}

// --- Channel status parser ---

std::vector<ChannelStatusDetails> PCFInquiry::parse_channel_status_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<ChannelStatusDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        ChannelStatusDetails ch;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 4) { // MQCFT_STRING
                    auto val = read_pcf_string(pdata, plen);
                    switch (pid) {
                    case 3501: ch.channel_name = val; break;
                    case 3506: ch.connection_name = val; break;
                    case 3507: ch.remote_qmgr = val; break;
                    case 3508: ch.job_name = val; break;
                    case 3544: ch.ssl_cipher = val; break;
                    }
                } else if (ptype == 3) { // MQCFT_INTEGER
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 1527: ch.status = val; break;
                    case 1521: ch.channel_type = val; break;
                    case 1501: ch.msgs = val; break;
                    case 1504: ch.batches = val; break;
                    case 1601: ch.substate = val; break;
                    case 1522: ch.instance_type = val; break;
                    }
                } else if (ptype == 23) { // MQCFT_INTEGER64
                    int64_t val = read_pcf_int64(pdata, plen);
                    switch (pid) {
                    case 1502: ch.bytes_sent = val; break;
                    case 1503: ch.bytes_received = val; break;
                    }
                }
            });
        if (!ch.channel_name.empty()) result.push_back(std::move(ch));
    }
    spdlog::debug("Parsed {} channel status entries", result.size());
    return result;
}

// --- Topic status parser ---

std::vector<TopicStatusDetails> PCFInquiry::parse_topic_status_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<TopicStatusDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        TopicStatusDetails topic;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 4) {
                    auto val = read_pcf_string(pdata, plen);
                    switch (pid) {
                    case 2094: topic.topic_string = val; break;
                    case 2092: topic.topic_name = val; break;
                    }
                } else if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 65: topic.topic_type = val; break;
                    case 88: topic.pub_count = val; break;
                    case 89: topic.sub_count = val; break;
                    }
                }
            });
        if (!topic.topic_string.empty() || !topic.topic_name.empty())
            result.push_back(std::move(topic));
    }
    spdlog::debug("Parsed {} topic status entries", result.size());
    return result;
}

// --- Subscription status parser ---

std::vector<SubStatusDetails> PCFInquiry::parse_sub_status_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<SubStatusDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        SubStatusDetails sub;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 4) {
                    auto val = read_pcf_string(pdata, plen);
                    switch (pid) {
                    case 2095: sub.sub_name = val; break;
                    case 2094: sub.topic_string = val; break;
                    case 1187: sub.destination = val; break;
                    case 7016: sub.sub_id = val; break;
                    }
                } else if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 71: sub.sub_type = val; break;
                    case 73: sub.durable = val; break;
                    }
                }
            });
        if (!sub.sub_name.empty()) result.push_back(std::move(sub));
    }
    spdlog::debug("Parsed {} subscription status entries", result.size());
    return result;
}

// --- QM status parser ---

std::vector<QMgrStatusDetails> PCFInquiry::parse_qmgr_status_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<QMgrStatusDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        QMgrStatusDetails qm;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 4) {
                    auto val = read_pcf_string(pdata, plen);
                    switch (pid) {
                    case 2002: qm.qmgr_name = val; break;
                    case 2003: qm.description = val; break;
                    case 3160: qm.start_date = val; break;
                    case 3161: qm.start_time = val; break;
                    }
                } else if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 119: qm.status = val; break;
                    case 120: qm.chinit_status = val; break;
                    case 121: qm.connection_count = val; break;
                    case 122: qm.cmd_server_status = val; break;
                    }
                }
            });
        result.push_back(std::move(qm));
    }
    spdlog::debug("Parsed {} QM status entries", result.size());
    return result;
}

// --- Cluster QM parser ---

std::vector<ClusterQMgrDetails> PCFInquiry::parse_cluster_qmgr_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<ClusterQMgrDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        ClusterQMgrDetails cl;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 4) {
                    auto val = read_pcf_string(pdata, plen);
                    switch (pid) {
                    case 2004: cl.cluster_name = val; break;
                    case 2002: cl.qmgr_name = val; break;
                    }
                } else if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 125:  cl.qm_type = val; break;
                    case 1127: cl.status = val; break;
                    }
                }
            });
        if (!cl.cluster_name.empty()) result.push_back(std::move(cl));
    }
    spdlog::debug("Parsed {} cluster QM entries", result.size());
    return result;
}

// --- Usage BP parser ---

std::vector<UsageBPDetails> PCFInquiry::parse_usage_bp_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<UsageBPDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        UsageBPDetails bp;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 22:   bp.buffer_pool = val; break;
                    case 1135: bp.free_buffers = val; break;
                    case 1136: bp.total_buffers = val; break;
                    case 1133: bp.location = val; break;
                    case 1134: bp.page_class = val; break;
                    }
                }
            });
        result.push_back(std::move(bp));
    }
    return result;
}

// --- Usage PS parser ---

std::vector<UsagePSDetails> PCFInquiry::parse_usage_ps_response(
        const std::vector<std::vector<uint8_t>>& responses) {
    std::vector<UsagePSDetails> result;
    for (const auto& resp : responses) {
        if (resp.size() < MQCFH_SIZE) continue;
        UsagePSDetails ps;
        parse_pcf_response_params(resp.data(), resp.size(),
            [&](uint32_t ptype, uint32_t pid, const uint8_t* pdata, uint32_t plen) {
                if (ptype == 3) {
                    int32_t val = read_pcf_int32(pdata, plen);
                    switch (pid) {
                    case 62:   ps.pageset_id = val; break;
                    case 22:   ps.buffer_pool = val; break;
                    case 1126: ps.total_pages = val; break;
                    case 1128: ps.unused_pages = val; break;
                    case 1129: ps.persist_pages = val; break;
                    case 1130: ps.nonpersist_pages = val; break;
                    case 1131: ps.restart_pages = val; break;
                    case 1132: ps.expand_count = val; break;
                    }
                }
            });
        result.push_back(std::move(ps));
    }
    return result;
}

// --- Existing queue status response parser ---
// Used by discover_queues() and get_queue_handle_details_by_pcf()

void PCFInquiry::parse_string_param(const uint8_t* data, size_t len,
                                    QueueHandleDetails& handle) {
    // data points past Type+StrucLength (at Parameter field)
    // Layout: Parameter(4) + CodedCharSetId(4) + StringLength(4) + String[N]
    if (len < 12) return;
    uint32_t param_id = read_int32(data);
    uint32_t str_len  = read_int32(data + 8);
    if (12 + str_len > len) return;
    auto val = trim_mq_string(std::string(reinterpret_cast<const char*>(data + 12), str_len));

    switch (param_id) {
    case 2016: handle.queue_name = val; break;
    case 3501: handle.channel_name = val; break;
    case 3502: handle.connection_name = val; break;
    case 2549: handle.application_tag = val; break;
    case 2046: handle.user_id = val; break;
    }
}

void PCFInquiry::parse_integer_param(const uint8_t* data, size_t len,
                                     QueueHandleDetails& handle) {
    // data points past Type+StrucLength (at Parameter field)
    // Layout: Parameter(4) + Value(4)
    if (len < 8) return;
    uint32_t param_id = read_int32(data);
    int32_t  value    = static_cast<int32_t>(read_int32(data + 4));

    switch (param_id) {
    case 3002: handle.process_id = value; break;
    case 1411: if (value > 0) handle.input_mode = "INPUT"; break;
    case 1412: if (value > 0) handle.output_mode = "OUTPUT"; break;
    }
}

std::vector<QueueHandleDetails> PCFInquiry::parse_queue_status_response(
        const uint8_t* data, size_t len) {
    std::vector<QueueHandleDetails> handles;
    if (len < MQCFH_SIZE) return handles;

    // CompCode at offset 24
    uint32_t comp_code = read_int32(data + 24);

    if (comp_code != 0) {
        spdlog::warn("PCF response error: comp_code={}, reason={}",
                     comp_code, read_int32(data + 28));
        return handles;
    }

    size_t offset = MQCFH_SIZE; // 36 bytes
    QueueHandleDetails current;
    bool in_group = false;

    while (offset + 8 <= len) {
        uint32_t param_type      = read_int32(data + offset);
        uint32_t param_struc_len = read_int32(data + offset + 4);
        if (param_struc_len == 0 || offset + param_struc_len > len) break;

        const uint8_t* param_data = data + offset + 8;
        size_t param_data_len = param_struc_len - 8;

        switch (param_type) {
        case 20: // MQCFT_GROUP
            if (in_group && !current.queue_name.empty())
                handles.push_back(current);
            current = QueueHandleDetails{};
            in_group = true;
            break;
        case 4: // MQCFT_STRING
            parse_string_param(param_data, param_data_len, current);
            break;
        case 3: // MQCFT_INTEGER
            parse_integer_param(param_data, param_data_len, current);
            break;
        }

        offset += param_struc_len;
    }

    // Handle non-grouped responses (INQUIRE_Q returns one queue per response message)
    if (!in_group && current.queue_name.empty()) {
        // Try to extract queue name from flat (non-grouped) parameters
        QueueHandleDetails flat;
        size_t off = MQCFH_SIZE;
        while (off + 8 <= len) {
            uint32_t ptype = read_int32(data + off);
            uint32_t plen  = read_int32(data + off + 4);
            if (plen == 0 || off + plen > len) break;
            if (ptype == 4) { // MQCFT_STRING
                parse_string_param(data + off + 8, plen - 8, flat);
            } else if (ptype == 3) { // MQCFT_INTEGER
                parse_integer_param(data + off + 8, plen - 8, flat);
            }
            off += plen;
        }
        if (!flat.queue_name.empty())
            handles.push_back(flat);
    } else if (in_group && !current.queue_name.empty()) {
        handles.push_back(current);
    }

    spdlog::info("Parsed {} handle details from PCF response", handles.size());
    return handles;
}

} // namespace ibmmq_exporter
