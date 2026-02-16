#include "ibmmq_exporter/resource_monitor.h"

#include <algorithm>
#include <cctype>
#include <cstring>

extern "C" {
#include <cmqc.h>
}

#include <spdlog/spdlog.h>

namespace ibmmq_exporter {

// PCF structure type constants for local parsing
namespace {
    constexpr int32_t PCF_INTEGER      = 3;
    constexpr int32_t PCF_STRING       = 4;
    constexpr int32_t PCF_INTEGER_LIST = 5;
    constexpr int32_t PCF_STRING_LIST  = 6;
    constexpr int32_t PCF_GROUP        = 20;
    constexpr int32_t PCF_INTEGER64    = 23;
    constexpr int32_t PCF_INTEGER64_LIST = 25;

    constexpr size_t PCF_HEADER_SIZE = 36; // MQCFH size
} // anonymous namespace

ResourceMonitor::ResourceMonitor(MQClient& client, const std::string& qmgr_name)
    : client_(client), qmgr_name_(qmgr_name) {}

std::vector<ResourceMonitor::PCFParam> ResourceMonitor::parse_pcf_params(
        const uint8_t* data, size_t len) {
    std::vector<PCFParam> params;
    size_t offset = 0;

    while (offset + 16 <= len) {
        int32_t type = 0, struc_len = 0, param_id = 0;
        std::memcpy(&type, data + offset, 4);
        std::memcpy(&struc_len, data + offset + 4, 4);
        std::memcpy(&param_id, data + offset + 8, 4);

        if (struc_len < 16 || offset + static_cast<size_t>(struc_len) > len) break;

        PCFParam p;
        p.type = type;
        p.param_id = param_id;

        if (type == PCF_INTEGER) {
            if (struc_len >= 16) {
                std::memcpy(&p.int_value, data + offset + 12, 4);
                p.int64_value = p.int_value;
            }
        } else if (type == PCF_STRING) {
            if (struc_len >= 20) {
                int32_t str_len = 0;
                std::memcpy(&str_len, data + offset + 16, 4);
                if (str_len > 0 && offset + 20 + static_cast<size_t>(str_len) <= len) {
                    p.str_value = std::string(
                        reinterpret_cast<const char*>(data + offset + 20), str_len);
                    auto pos = p.str_value.find_last_not_of(" \0");
                    if (pos != std::string::npos) p.str_value.resize(pos + 1);
                    else p.str_value.clear();
                }
            }
        } else if (type == PCF_INTEGER64) {
            if (struc_len >= 24) {
                std::memcpy(&p.int64_value, data + offset + 16, 8);
                p.int_value = static_cast<int32_t>(p.int64_value);
            }
        } else if (type == PCF_GROUP) {
            int32_t group_count = 0;
            std::memcpy(&group_count, data + offset + 12, 4);

            // Parse nested parameters inside the group
            size_t inner_offset = offset + 16;
            for (int32_t i = 0; i < group_count && inner_offset < offset + static_cast<size_t>(struc_len); ++i) {
                if (inner_offset + 16 > len) break;

                int32_t inner_type = 0, inner_len = 0, inner_param = 0;
                std::memcpy(&inner_type, data + inner_offset, 4);
                std::memcpy(&inner_len, data + inner_offset + 4, 4);
                std::memcpy(&inner_param, data + inner_offset + 8, 4);

                if (inner_len < 16 || inner_offset + static_cast<size_t>(inner_len) > len) break;

                PCFParam inner;
                inner.type = inner_type;
                inner.param_id = inner_param;

                if (inner_type == PCF_INTEGER) {
                    std::memcpy(&inner.int_value, data + inner_offset + 12, 4);
                    inner.int64_value = inner.int_value;
                } else if (inner_type == PCF_STRING) {
                    if (inner_len >= 20) {
                        int32_t str_len = 0;
                        std::memcpy(&str_len, data + inner_offset + 16, 4);
                        if (str_len > 0 && inner_offset + 20 + static_cast<size_t>(str_len) <= len) {
                            inner.str_value = std::string(
                                reinterpret_cast<const char*>(data + inner_offset + 20), str_len);
                            auto pos = inner.str_value.find_last_not_of(" \0");
                            if (pos != std::string::npos) inner.str_value.resize(pos + 1);
                            else inner.str_value.clear();
                        }
                    }
                } else if (inner_type == PCF_INTEGER64) {
                    if (inner_len >= 24) {
                        std::memcpy(&inner.int64_value, data + inner_offset + 16, 8);
                        inner.int_value = static_cast<int32_t>(inner.int64_value);
                    }
                }

                p.group_params.push_back(std::move(inner));
                inner_offset += static_cast<size_t>(inner_len);
            }
        }

        params.push_back(std::move(p));
        offset += static_cast<size_t>(struc_len);
    }
    return params;
}

bool ResourceMonitor::discover() {
    spdlog::info("Starting resource monitor metadata discovery for QM '{}'", qmgr_name_);

    if (!discover_classes()) {
        spdlog::warn("Failed to discover monitor classes");
        return false;
    }

    spdlog::info("Discovered {} monitor classes", classes_.size());
    for (const auto& cls : classes_) {
        spdlog::info("  Class '{}' ({}) with {} types",
                     cls.class_name, cls.class_id, cls.types.size());
        for (const auto& t : cls.types) {
            spdlog::debug("    Type '{}' ({}) with {} elements, topic: {}",
                         t.type_name, t.type_id, t.elements.size(), t.object_topic);
        }
    }

    // Build element lookup map
    for (const auto& cls : classes_) {
        for (const auto& t : cls.types) {
            for (const auto& e : t.elements) {
                element_map_[e.element_id] = {cls.class_name, t.type_name, &e};
            }
        }
    }

    spdlog::info("Built element lookup map with {} entries", element_map_.size());
    return true;
}

bool ResourceMonitor::discover_classes() {
    std::string classes_topic = "$SYS/MQ/INFO/QMGR/" + qmgr_name_ + "/Monitor/METADATA/CLASSES";
    spdlog::info("Subscribing to metadata topic: {}", classes_topic);

    auto msg = client_.subscribe_and_get(classes_topic);
    if (!msg) {
        spdlog::warn("No response from CLASSES metadata topic for QM '{}'", qmgr_name_);
        spdlog::warn("Resource monitoring publications require MONQ and MONCHL to be enabled on the queue manager.");
        spdlog::warn("Run the following commands on the queue manager to enable resource monitoring:");
        spdlog::warn("  ALTER QMGR MONQ(MEDIUM) MONCHL(MEDIUM)");
        spdlog::warn("  ALTER QMGR ACTVTRC(ON)");
        spdlog::warn("Note: STATMQI/STATQ enable statistics messages to admin queues (a different feature).");
        spdlog::warn("After changing MONQ/MONCHL, the queue manager may need to be restarted.");
        return false;
    }

    const auto& data = msg->data;
    spdlog::info("Received CLASSES metadata: {} bytes", data.size());
    if (data.size() < PCF_HEADER_SIZE) return false;

    // Dump MQCFH header fields for diagnostics
    int32_t cfh_type = 0, cfh_struc_length = 0, cfh_version = 0, cfh_command = 0;
    int32_t cfh_compcode = 0, cfh_reason = 0, cfh_paramcount = 0;
    std::memcpy(&cfh_type, data.data(), 4);
    std::memcpy(&cfh_struc_length, data.data() + 4, 4);
    std::memcpy(&cfh_version, data.data() + 8, 4);
    std::memcpy(&cfh_command, data.data() + 12, 4);
    std::memcpy(&cfh_compcode, data.data() + 24, 4);
    std::memcpy(&cfh_reason, data.data() + 28, 4);
    std::memcpy(&cfh_paramcount, data.data() + 32, 4);

    spdlog::info("MQCFH: Type={}, StrucLength={}, Version={}, Command={}, CompCode={}, Reason={}, ParamCount={}",
                 cfh_type, cfh_struc_length, cfh_version, cfh_command, cfh_compcode, cfh_reason, cfh_paramcount);

    size_t params_offset = (cfh_struc_length >= 36 && static_cast<size_t>(cfh_struc_length) <= data.size())
                           ? static_cast<size_t>(cfh_struc_length) : PCF_HEADER_SIZE;

    auto params = parse_pcf_params(data.data() + params_offset, data.size() - params_offset);
    spdlog::info("Parsed {} top-level PCF parameters from CLASSES metadata", params.size());

    // Log all parameter types for diagnostics
    for (size_t i = 0; i < params.size(); ++i) {
        const auto& p = params[i];
        if (p.type == PCF_STRING) {
            spdlog::info("  Param[{}]: type={} (STRING), id={}, str='{}'", i, p.type, p.param_id, p.str_value);
        } else if (p.type == PCF_INTEGER) {
            spdlog::info("  Param[{}]: type={} (INT), id={}, val={}", i, p.type, p.param_id, p.int_value);
        } else if (p.type == PCF_GROUP) {
            spdlog::info("  Param[{}]: type={} (GROUP), id={}, inner_count={}", i, p.type, p.param_id, p.group_params.size());
        } else {
            spdlog::info("  Param[{}]: type={}, id={}", i, p.type, p.param_id);
        }
    }

    // Each group represents a class
    for (const auto& p : params) {
        if (p.type != PCF_GROUP) continue;

        MonitorClass cls;
        for (const auto& gp : p.group_params) {
            if (gp.param_id == monitor_pcf::MQIAMO_MONITOR_CLASS)
                cls.class_id = gp.int_value;
            else if (gp.param_id == monitor_pcf::MQCAMO_MONITOR_CLASS)
                cls.class_name = gp.str_value;
            else if (gp.param_id == monitor_pcf::MQCA_TOPIC_STRING)
                cls.topic_string = gp.str_value;
        }

        if (cls.class_name.empty()) continue;

        spdlog::info("Found monitor class '{}' (id={}), types topic: {}",
                     cls.class_name, cls.class_id, cls.topic_string);

        // Discover types for this class
        if (discover_types(cls)) {
            classes_.push_back(std::move(cls));
        }
    }

    return !classes_.empty();
}

bool ResourceMonitor::discover_types(MonitorClass& cls) {
    if (cls.topic_string.empty()) return false;

    auto msg = client_.subscribe_and_get(cls.topic_string);
    if (!msg) {
        spdlog::warn("No response from types topic for class '{}'", cls.class_name);
        return false;
    }

    const auto& data = msg->data;
    if (data.size() < PCF_HEADER_SIZE) return false;

    int32_t cfh_struc_length = 0;
    std::memcpy(&cfh_struc_length, data.data() + 4, 4);
    size_t params_offset = (cfh_struc_length > 0 && static_cast<size_t>(cfh_struc_length) <= data.size())
                           ? static_cast<size_t>(cfh_struc_length) : PCF_HEADER_SIZE;

    auto params = parse_pcf_params(data.data() + params_offset, data.size() - params_offset);

    for (const auto& p : params) {
        if (p.type != PCF_GROUP) continue;

        MonitorType mtype;
        std::string elements_topic;

        for (const auto& gp : p.group_params) {
            if (gp.param_id == monitor_pcf::MQIAMO_MONITOR_TYPE)
                mtype.type_id = gp.int_value;
            else if (gp.param_id == monitor_pcf::MQCAMO_MONITOR_TYPE)
                mtype.type_name = gp.str_value;
            else if (gp.param_id == monitor_pcf::MQCA_TOPIC_STRING)
                elements_topic = gp.str_value;
        }

        if (mtype.type_name.empty() || elements_topic.empty()) continue;

        spdlog::info("  Type '{}' (id={}) in class '{}', elements topic: {}",
                     mtype.type_name, mtype.type_id, cls.class_name, elements_topic);

        // Discover elements by subscribing to the elements metadata topic
        discover_elements(cls, mtype, elements_topic);

        if (!mtype.elements.empty())
            cls.types.push_back(std::move(mtype));
    }

    return !cls.types.empty();
}

bool ResourceMonitor::discover_elements(MonitorClass& cls, MonitorType& mtype,
                                         const std::string& elements_topic) {
    auto msg = client_.subscribe_and_get(elements_topic);
    if (!msg) {
        spdlog::warn("No response from elements topic '{}' for {}/{}", elements_topic, cls.class_name, mtype.type_name);
        return false;
    }

    const auto& data = msg->data;
    if (data.size() < PCF_HEADER_SIZE) return false;

    int32_t cfh_struc_length = 0;
    std::memcpy(&cfh_struc_length, data.data() + 4, 4);
    size_t params_offset = (cfh_struc_length > 0 && static_cast<size_t>(cfh_struc_length) <= data.size())
                           ? static_cast<size_t>(cfh_struc_length) : PCF_HEADER_SIZE;

    auto params = parse_pcf_params(data.data() + params_offset, data.size() - params_offset);

    for (const auto& p : params) {
        // The ObjectTopic (data publication topic) is a top-level string param
        if (p.type == PCF_STRING && p.param_id == monitor_pcf::MQCA_TOPIC_STRING) {
            mtype.object_topic = p.str_value;
            continue;
        }

        if (p.type != PCF_GROUP) continue;

        MonitorElement elem;
        for (const auto& gp : p.group_params) {
            if (gp.param_id == monitor_pcf::MQIAMO_MONITOR_ELEMENT)
                elem.element_id = gp.int_value;
            else if (gp.param_id == monitor_pcf::MQIAMO_MONITOR_DATATYPE)
                elem.datatype = gp.int_value;
            else if (gp.param_id == monitor_pcf::MQCAMO_MONITOR_DESC)
                elem.description = gp.str_value;
        }

        if (elem.element_id == 0) continue;

        elem.metric_name = description_to_metric_name(elem.description);

        spdlog::debug("    Element {} '{}' -> metric '{}', datatype={}",
                     elem.element_id, elem.description, elem.metric_name, elem.datatype);

        mtype.elements.push_back(std::move(elem));
    }

    return !mtype.elements.empty();
}

bool ResourceMonitor::create_subscriptions() {
    if (classes_.empty()) {
        spdlog::warn("No monitor classes discovered, skipping subscription creation");
        return false;
    }

    // Subscribe to wildcard data topics for each class
    for (const auto& cls : classes_) {
        std::string data_topic = "$SYS/MQ/INFO/QMGR/" + qmgr_name_ + "/Monitor/" + cls.class_name + "/#";
        client_.subscribe_to_topic(data_topic);
        spdlog::info("Subscribed to data topic: {}", data_topic);
    }

    spdlog::info("Created {} data subscriptions for resource monitoring", classes_.size());
    return true;
}

std::vector<PublicationMetric> ResourceMonitor::process_publications() {
    std::vector<PublicationMetric> metrics;

    auto messages = client_.receive_publications();
    if (messages.empty()) return metrics;

    spdlog::debug("Processing {} publication messages", messages.size());

    for (const auto& msg : messages) {
        if (msg.data.size() < PCF_HEADER_SIZE) continue;

        // Read actual MQCFH StrucLength
        int32_t cfh_struc_length = 0;
        std::memcpy(&cfh_struc_length, msg.data.data() + 4, 4);
        size_t params_offset = (cfh_struc_length > 0 && static_cast<size_t>(cfh_struc_length) <= msg.data.size())
                               ? static_cast<size_t>(cfh_struc_length) : PCF_HEADER_SIZE;

        auto params = parse_pcf_params(
            msg.data.data() + params_offset,
            msg.data.size() - params_offset);

        // Extract QM name and queue name from top-level params
        std::string queue_name;

        for (const auto& p : params) {
            if (p.param_id == monitor_pcf::MQCA_Q_NAME)
                queue_name = p.str_value;
        }

        // Process int and int64 parameters as metrics
        for (const auto& p : params) {
            if (p.type != PCF_INTEGER64 && p.type != PCF_INTEGER) continue;

            auto it = element_map_.find(p.param_id);
            if (it == element_map_.end()) continue;

            const auto& lookup = it->second;
            const auto* elem = lookup.element;
            if (!elem) continue;

            PublicationMetric pm;
            pm.class_name = lookup.class_name;
            pm.type_name = lookup.type_name;
            pm.object_name = queue_name;
            pm.metric_name = elem->metric_name;
            pm.value = normalize_value(p.int64_value, elem->datatype);
            pm.is_delta = (elem->datatype == monitor_datatype::DELTA);
            metrics.push_back(std::move(pm));
        }
    }

    if (!metrics.empty())
        spdlog::debug("Processed {} publication metrics", metrics.size());

    return metrics;
}

void ResourceMonitor::close() {
    data_subs_.clear();
    element_map_.clear();
    classes_.clear();
}

std::string ResourceMonitor::description_to_metric_name(const std::string& desc) {
    std::string result;
    result.reserve(desc.size());

    bool prev_underscore = false;
    for (char c : desc) {
        if (c == ' ' || c == '-' || c == '/' || c == '(' || c == ')') {
            if (!result.empty() && !prev_underscore) {
                result += '_';
                prev_underscore = true;
            }
        } else if (std::isalnum(static_cast<unsigned char>(c))) {
            result += static_cast<char>(std::tolower(static_cast<unsigned char>(c)));
            prev_underscore = false;
        }
    }

    // Trim trailing underscore
    if (!result.empty() && result.back() == '_')
        result.pop_back();

    return result;
}

double ResourceMonitor::normalize_value(int64_t raw, int32_t datatype) {
    double val = static_cast<double>(raw);
    switch (datatype) {
    case monitor_datatype::HUNDREDTHS:
        return val / 100.0;
    case monitor_datatype::PERCENT:
        return val / 100.0;  // stored as hundredths of percent
    case monitor_datatype::MICROSEC:
        return val / 1000000.0;  // convert to seconds
    default:
        return val;  // UNIT, DELTA, KB, MB, GB — keep raw
    }
}

std::string ResourceMonitor::extract_object_name(const std::string& topic_string,
                                                  const std::string& class_name) {
    if (class_name != "STATQ") return "";

    auto pos = topic_string.rfind('/');
    if (pos != std::string::npos && pos + 1 < topic_string.size()) {
        return topic_string.substr(pos + 1);
    }
    return "";
}

} // namespace ibmmq_exporter
