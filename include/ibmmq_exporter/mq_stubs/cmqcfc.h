/* Stub IBM MQ PCF header */
#ifndef CMQCFC_H_STUB
#define CMQCFC_H_STUB
#include "cmqc.h"

/* PCF structure types */
#define MQCFT_NONE               0
#define MQCFT_COMMAND            1
#define MQCFT_RESPONSE           2
#define MQCFT_INTEGER            3
#define MQCFT_STRING             4
#define MQCFT_INTEGER_LIST       5
#define MQCFT_STRING_LIST        6
#define MQCFT_GROUP              20
#define MQCFT_STATISTICS         21
#define MQCFT_ACCOUNTING         22
#define MQCFT_INTEGER64          23
#define MQCFT_INTEGER64_LIST     25
#define MQCFT_COMMAND_XR         16

/* PCF commands */
#define MQCMD_INQUIRE_Q                 3
#define MQCMD_RESET_Q_STATS             17
#define MQCMD_INQUIRE_Q_STATUS          34
#define MQCMD_INQUIRE_CHANNEL_STATUS    41
#define MQCMD_INQUIRE_CLUSTER_Q_MGR     71
#define MQCMD_INQUIRE_USAGE             84
#define MQCMD_INQUIRE_TOPIC_STATUS      87
#define MQCMD_INQUIRE_SUB_STATUS        92
#define MQCMD_STATISTICS_MQI            112
#define MQCMD_STATISTICS_Q              113
#define MQCMD_STATISTICS_CHANNEL        114
#define MQCMD_ACCOUNTING_MQI            138
#define MQCMD_ACCOUNTING_Q              139
#define MQCMD_INQUIRE_Q_MGR_STATUS      161
#define MQCMD_Q_MGR_STATUS              164
#define MQCMD_ACCOUNTING_CHANNEL        167

/* PCF control */
#define MQCFC_LAST               1
#define MQCFC_NOT_LAST           0

/* Channel status parameter IDs */
#define MQCACH_CHANNEL_NAME         3501
#define MQCACH_CONNECTION_NAME      3506
#define MQCACH_REMOTE_Q_MGR_NAME    3507
#define MQCACH_SSL_CIPHER_SPEC      3544
#define MQCACH_JOB_NAME             3508
#define MQIACH_CHANNEL_STATUS       1527
#define MQIACH_CHANNEL_TYPE         1521
#define MQIACH_MSGS                 1501
#define MQIACH_BYTES_SENT           1502
#define MQIACH_BYTES_RECEIVED       1503
#define MQIACH_BATCHES              1504
#define MQIACH_CHANNEL_SUBSTATE     1601
#define MQIACH_CHANNEL_INSTANCE_TYPE 1522

/* Channel type values */
#define MQCHT_SENDER     1
#define MQCHT_SERVER     2
#define MQCHT_RECEIVER   3
#define MQCHT_REQUESTER  4
#define MQCHT_SVRCONN    7
#define MQCHT_CLNTCONN   6
#define MQCHT_CLUSSDR    8
#define MQCHT_CLUSRCVR   9
#define MQCHT_AMQP       14
#define MQCHT_MQTT       15

/* Channel status values */
#define MQCHS_INACTIVE   0
#define MQCHS_BINDING    1
#define MQCHS_STARTING   2
#define MQCHS_RUNNING    3
#define MQCHS_STOPPING   4
#define MQCHS_RETRYING   5
#define MQCHS_STOPPED    6
#define MQCHS_REQUESTING 7
#define MQCHS_PAUSED     8
#define MQCHS_DISCONNECTED 9
#define MQCHS_INITIALIZING 13
#define MQCHS_SWITCHING  26

/* Topic status parameter IDs */
#define MQCA_TOPIC_STRING       2094
#define MQCA_TOPIC_NAME         2092
#define MQIA_TOPIC_TYPE         65
#define MQIA_PUB_COUNT          88
#define MQIA_SUB_COUNT          89
#define MQIACF_TOPIC_STATUS_TYPE 1185

/* Topic status type values */
#define MQIACF_TOPIC_SUB        1
#define MQIACF_TOPIC_PUB        2
#define MQIACF_TOPIC_STATUS     0

/* Subscription status parameter IDs */
#define MQCA_SUB_NAME           2095
#define MQBACF_SUB_ID           7016
#define MQIA_SUB_TYPE           71
#define MQIA_DURABLE_SUB        73
#define MQIACF_SUB_STATUS_TYPE  1186
#define MQIACF_DESTINATION      1187

/* QM status parameter IDs */
#define MQCA_Q_MGR_NAME         2002
#define MQCA_Q_MGR_DESC         2003
#define MQIA_Q_MGR_STATUS       119
#define MQIA_CHINIT_STATUS      120
#define MQIA_CONNS              121
#define MQIA_CMD_SERVER_STATUS  122
#define MQCACF_Q_MGR_START_DATE 3160
#define MQCACF_Q_MGR_START_TIME 3161

/* Cluster QM parameter IDs */
#define MQCA_CLUSTER_NAME       2004
#define MQIA_QM_TYPE            125
#define MQIACF_CLUSTER_Q_MGR_STATUS 1127

/* z/OS usage parameter IDs */
#define MQIACF_USAGE_TYPE       1125
#define MQIACF_USAGE_BP         1
#define MQIACF_USAGE_PS         2
#define MQIA_BUFFER_POOL        22
#define MQIA_PAGESET_ID         62
#define MQIACF_USAGE_TOTAL_PAGES     1126
#define MQIACF_USAGE_UNUSED_PAGES    1128
#define MQIACF_USAGE_PERSIST_PAGES   1129
#define MQIACF_USAGE_NONPERSIST_PAGES 1130
#define MQIACF_USAGE_RESTART_PAGES   1131
#define MQIACF_USAGE_EXPAND_COUNT    1132
#define MQIACF_USAGE_LOCATION        1133
#define MQIACF_USAGE_PAGE_CLASS      1134
#define MQIACF_USAGE_FREE_BUFF       1135
#define MQIACF_USAGE_TOTAL_BUFF      1136

/* Queue attributes for INQUIRE_Q */
#define MQCA_Q_NAME             2016
#define MQCA_Q_DESC             2017
#define MQIA_Q_TYPE             20
#define MQIA_CURRENT_Q_DEPTH    3
#define MQIA_MAX_Q_DEPTH        15

/* PCF Header - MQCFH */
typedef struct tagMQCFH {
    MQLONG Type;
    MQLONG StrucLength;
    MQLONG Version;
    MQLONG Command;
    MQLONG MsgSeqNumber;
    MQLONG Control;
    MQLONG CompCode;
    MQLONG Reason;
    MQLONG ParameterCount;
} MQCFH;

/* PCF String Parameter - MQCFST */
typedef struct tagMQCFST {
    MQLONG Type;
    MQLONG StrucLength;
    MQLONG Parameter;
    MQLONG CodedCharSetId;
    MQLONG StringLength;
    MQCHAR String[1]; /* variable length */
} MQCFST;

/* PCF Integer Parameter - MQCFIN */
typedef struct tagMQCFIN {
    MQLONG Type;
    MQLONG StrucLength;
    MQLONG Parameter;
    MQLONG Value;
} MQCFIN;

/* PCF Integer64 Parameter */
typedef struct tagMQCFIN64 {
    MQLONG  Type;
    MQLONG  StrucLength;
    MQLONG  Parameter;
    MQLONG  Reserved;
    MQINT64 Value;
} MQCFIN64;

#endif /* CMQCFC_H_STUB */
