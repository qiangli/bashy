/* Hosted Windows only. Prima no-egress gate: a disposable dynamic WFP policy
 * explicitly permits exactly the pinned prima loopback ports for the exercise
 * processes, then spawns the multi-process exercise (exercise.py driving the
 * CLI, two server ranks and the ring test) inside a Windows job object so
 * cleanup is unconditional.
 *
 * Enforcement model (per protected executable, by exact file path):
 *   - IPv4 loopback (127.0.0.1) to the pinned allowlist explicitly
 *     PERMITted, one filter per contiguous range;
 *   - IPv6 completely denied;
 *   - IPv4 to any remote address other than 127.0.0.1 denied;
 *   - TCP/UDP ports outside the pinned allowlist denied via four filters
 *     that tile the complement of the allowlist over [0, 65535].
 * A WFP BLOCK veto wins over any PERMIT regardless of weight, so the
 * explicit loopback permits cannot weaken the denies: traffic the denies
 * match stays denied, and allowed loopback is decided by a rule instead of
 * by the absence of one. WFP ALE_AUTH_CONNECT covers TCP, UDP and
 * non-error ICMP, including loopback, so loopback traffic IS filtered
 * (Windows Firewall would not be).
 * The dynamic session exists before CreateProcess and is removed after the
 * reap; the job owns cleanup with KILL_ON_JOB_CLOSE. There is deliberately
 * no single-process limit: the exercise runs several cooperating processes.
 *
 * Diagnostics: every control names its endpoint on failure and dumps the
 * local bound TCP table (IPv4 loopback vs IPv6), and the same dump runs
 * when the exercise child fails or exceeds the outer deadline, so the next
 * failure says what the exercise actually bound. Dumps never fail the gate.
 * The external control targets unroutable TEST-NET-1, which never answers,
 * so a missing ALE block reads as silence (select timeout), not as a fast
 * verdict: CI run 38472050604 timed out there with no explicit deny. A
 * BLOCK at ALE_AUTH_CONNECT surfaces as WSAEACCES, never as silence, so a
 * single attempt is classified (wfp-verdict.h) and silence passes the
 * control ONLY when a CLASSIFY_DROP net event proves OUR deny filter
 * matched the flow; silence with no such event means the filter missed and
 * the runner fabric dropped the SYN, and fails with that diagnosis.
 * Retrying the silence cannot tell those apart, so the external control
 * does not retry. The fast loopback/IPv6 denies run first: their WSAEACCES
 * proves app-id matching for this process before the external verdict is
 * read. The real exercise stays strict: any denial past the marker fails.
 *
 * Pinned allowlist (prima 0.1.x ring + engine ports; keep in sync with
 * no-egress-test.sh and exercise.py):
 *   ring 5599-5605, engine data 19000/19001, engine signal 19100/19101.
 *
 * Usage: guard.exe --app EXE [--app EXE ...] -- PIDFILE EXECUTABLE [ARGS...]
 * Controls print directly-observed verdicts (WSAEACCES / delivery proof,
 * plus the deny-filter inventory and -- for the external control only -- a
 * net-event-proven silent-drop) to stdout BEFORE the marker line
 * "WFP exercise filters installed"; the gate script splits the log there
 * and refuses denial signatures after it.
 */
#define _WIN32_WINNT 0x0602
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <fwpmu.h>
#include <iphlpapi.h>
#include <rpc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include "wfp-verdict.h"

/* The classifier's Winsock values must stay bound to the real ones; drift
 * would fork the Linux-tested verdict table from the Windows gate. */
_Static_assert(WSAEACCES == WFP_WSAEACCES, "WSAEACCES binding");
_Static_assert(WSAEWOULDBLOCK == WFP_WSAEWOULDBLOCK, "WSAEWOULDBLOCK binding");
_Static_assert(WSAECONNREFUSED == WFP_WSAECONNREFUSED, "WSAECONNREFUSED binding");

static const GUID connect4 = {0xc38d57d1,0x05a7,0x4c33,{0x90,0x4f,0x7f,0xbc,0xee,0xe6,0x0e,0x82}};
static const GUID connect6 = {0x4a72393b,0x319f,0x44bc,{0x84,0xc3,0xba,0x54,0xdc,0xb3,0xb6,0xb4}};
static const GUID app_id = {0xd78e1e87,0x8644,0x4ea5,{0x94,0x37,0xd8,0x09,0xec,0xef,0xc9,0x71}};
static const GUID remote_address = {0xb235ae9a,0x1d64,0x49b8,{0xa4,0x4c,0x5f,0xf3,0xd9,0x09,0x50,0x45}};
static const GUID remote_port = {0xc35a604d,0xd22b,0x4e1a,{0x91,0xb4,0x68,0xf6,0x74,0xee,0x67,0x4b}};

static const UINT16 allowed_ports[] = {5599, 5600, 5601, 5602, 5603, 5604, 5605,
                                       19000, 19001, 19100, 19101};
/* Contiguous ranges tiling allowed_ports; one explicit PERMIT filter each. */
static const UINT16 allowed_ranges[][2] = {{5599, 5605}, {19000, 19001},
                                           {19100, 19101}};
/* Tiles of the complement of allowed_ports over [0, 65535]. */
static const UINT16 blocked_ranges[][2] = {{0, 5598}, {5606, 18999},
                                           {19002, 19099}, {19102, 65535}};
_Static_assert(sizeof(allowed_ports) / sizeof(allowed_ports[0]) == 11,
               "pinned allowlist size");
_Static_assert((5605u - 5599u + 1u) + (19001u - 19000u + 1u) + (19101u - 19100u + 1u) == 11,
               "permit ranges tile the allowlist");
/* Loopback/IPv6 denies land synchronously (no network involved), so one
 * attempt with a generous select fallback is enough; the external control
 * below gets its own short deadline because a matched ALE block is
 * immediate while an unmatched SYN to TEST-NET-1 stays silent. */
#define CONTROL_DEADLINE_SEC 15
#define EXTERNAL_DEADLINE_SEC 5
#define DATAGRAM_DEADLINE_SEC 5

/* Deny filter IDs captured from FwpmFilterAdd0: the "proven installed"
 * half of the external silent-drop acceptance (the "proven matched" half
 * is a CLASSIFY_DROP net event carrying one of these IDs). */
/* Six deny IDs per protect() call (IPv6 + non-loopback + four range
 * tiles); protect() runs once per guarded executable (self, the exercise
 * image, and each --app image), so size for several rounds with headroom.
 * The installed-verification and the external control both run while only
 * self's six are recorded. */
#define MAX_DENY_FILTERS 64
static UINT64 deny_ids[MAX_DENY_FILTERS];
static size_t deny_count = 0;

static void record_deny_id(UINT64 id) {
    if (deny_count >= MAX_DENY_FILTERS) {
        fprintf(stderr, "WFP gate failure: deny filter table overflow\n");
        exit(1);
    }
    deny_ids[deny_count++] = id;
}

static void checked(DWORD status, const char *operation) {
    if (status) {
        fprintf(stderr, "WFP gate failure: %s: %lu\n", operation, (unsigned long)status);
        exit(1);
    }
}
static void wincheck(BOOL ok, const char *operation) {
    if (!ok) checked(GetLastError() ? GetLastError() : 1, operation);
}

static void protect(HANDLE engine, GUID sublayer, const wchar_t *exe) {
    FWP_BYTE_BLOB *app = NULL;
    checked(FwpmGetAppIdFromFileName0(exe, &app), "app identity");
    FWPM_FILTER_CONDITION0 conditions[4] = {0};
    conditions[0].fieldKey = app_id;
    conditions[0].matchType = FWP_MATCH_EQUAL;
    conditions[0].conditionValue.type = FWP_BYTE_BLOB_TYPE;
    conditions[0].conditionValue.byteBlob = app;
    FWPM_FILTER0 filter = {0};
    filter.displayData.name = L"Bashy hosted prima no-egress";
    filter.subLayerKey = sublayer;
    filter.weight.type = FWP_EMPTY;
    filter.filterCondition = conditions;
    /* Explicit loopback allowlist: 127.0.0.1 within the pinned ranges is
     * PERMITted, one filter per contiguous range. A BLOCK veto wins over any
     * PERMIT, so these permits cannot weaken the denies below; they decide
     * allowed loopback explicitly instead of leaving it to the absence of
     * a block. */
    filter.action.type = FWP_ACTION_PERMIT;
    filter.layerKey = connect4;
    for (size_t i = 0; i < sizeof(allowed_ranges) / sizeof(allowed_ranges[0]); i++) {
        conditions[1].fieldKey = remote_address;
        conditions[1].matchType = FWP_MATCH_EQUAL;
        conditions[1].conditionValue.type = FWP_UINT32;
        conditions[1].conditionValue.uint32 = 0x7f000001; /* WFP host byte order */
        conditions[2].fieldKey = remote_port;
        conditions[2].matchType = FWP_MATCH_GREATER_OR_EQUAL;
        conditions[2].conditionValue.type = FWP_UINT16;
        conditions[2].conditionValue.uint16 = allowed_ranges[i][0];
        conditions[3].fieldKey = remote_port;
        conditions[3].matchType = FWP_MATCH_LESS_OR_EQUAL;
        conditions[3].conditionValue.type = FWP_UINT16;
        conditions[3].conditionValue.uint16 = allowed_ranges[i][1];
        filter.numFilterConditions = 4;
        checked(FwpmFilterAdd0(engine, &filter, NULL, NULL), "loopback allowlist permit");
    }
    filter.action.type = FWP_ACTION_BLOCK;
    /* IPv6 completely denied: every configured endpoint is IPv4 loopback. */
    filter.layerKey = connect6;
    filter.numFilterConditions = 1;
    {
        UINT64 id = 0;
        checked(FwpmFilterAdd0(engine, &filter, &id, NULL), "IPv6 deny");
        record_deny_id(id);
    }
    filter.layerKey = connect4;
    filter.numFilterConditions = 2;
    conditions[1].fieldKey = remote_address;
    conditions[1].matchType = FWP_MATCH_NOT_EQUAL;
    conditions[1].conditionValue.type = FWP_UINT32;
    conditions[1].conditionValue.uint32 = 0x7f000001; /* WFP host byte order */
    {
        UINT64 id = 0;
        checked(FwpmFilterAdd0(engine, &filter, &id, NULL), "non-loopback deny");
        record_deny_id(id);
    }
    /* Ports outside the pinned allowlist denied, one range filter each. */
    conditions[1].fieldKey = remote_port;
    conditions[1].matchType = FWP_MATCH_GREATER_OR_EQUAL;
    conditions[1].conditionValue.type = FWP_UINT16;
    conditions[2].fieldKey = remote_port;
    conditions[2].matchType = FWP_MATCH_LESS_OR_EQUAL;
    conditions[2].conditionValue.type = FWP_UINT16;
    filter.numFilterConditions = 3;
    for (size_t i = 0; i < sizeof(blocked_ranges) / sizeof(blocked_ranges[0]); i++) {
        UINT64 id = 0;
        conditions[1].conditionValue.uint16 = blocked_ranges[i][0];
        conditions[2].conditionValue.uint16 = blocked_ranges[i][1];
        checked(FwpmFilterAdd0(engine, &filter, &id, NULL), "outside-allowlist deny");
        record_deny_id(id);
    }
    FwpmFreeMemory0((void **)&app);
}

/* "Proven installed": every deny filter ID captured above must still
 * resolve by ID inside our sublayer (direct lookup, no enumeration
 * pagination to get wrong). Runs after protect(self) and before any
 * control, so a later silent external verdict is read against confirmed
 * policy, not assumed policy. */
static void verify_deny_filters_installed(HANDLE engine, const GUID *sublayer) {
    size_t found = 0;
    for (size_t k = 0; k < deny_count; k++) {
        FWPM_FILTER0 *filter = NULL;
        DWORD status = FwpmFilterGetById0(engine, deny_ids[k], &filter);
        if (status) {
            fprintf(stderr, "WFP gate failure: deny filter lookup %llu: %lu\n",
                    (unsigned long long)deny_ids[k], (unsigned long)status);
            exit(1);
        }
        if (memcmp(&filter->subLayerKey, sublayer, sizeof(GUID)) == 0)
            found++;
        FwpmFreeMemory0((void **)&filter);
    }
    printf("WFP deny filters installed count=%u matched=%u\n",
           (unsigned)deny_count, (unsigned)found);
    fflush(stdout);
    if (found != deny_count) {
        fprintf(stderr, "WFP gate failure: deny filters missing (%u of %u in our sublayer)\n",
                (unsigned)found, (unsigned)deny_count);
        exit(1);
    }
}

/* "Proven matched": scan recent CLASSIFY_DROP net events for one carrying
 * our ALE_CONNECT_V4 deny filter ID at or after 'since'. Returns the
 * matching filter ID, or 0 when no such event exists -- the run-38472050604
 * silence with no event here is the filter-miss/runner-drop case (b), not
 * a slow verdict, and must fail, not pass. */
static UINT64 external_drop_matched(HANDLE engine, const FILETIME *since) {
    FWPM_NET_EVENT_ENUM_TEMPLATE0 enum_template = {0};
    HANDLE enum_handle = NULL;
    checked(FwpmNetEventCreateEnumHandle0(engine, &enum_template, &enum_handle),
            "net event enum handle");
    FWPM_NET_EVENT0 **events = NULL;
    UINT32 returned = 0;
    checked(FwpmNetEventEnum0(engine, enum_handle, 256, &events, &returned),
            "net event enum");
    UINT64 matched = 0;
    for (UINT32 i = 0; i < returned && !matched; i++) {
        FWPM_NET_EVENT0 *event = events[i];
        if (event->type != FWPM_NET_EVENT_TYPE_CLASSIFY_DROP)
            continue;
        if (!event->classifyDrop)
            continue;
        if (CompareFileTime(&event->header.timeStamp, since) < 0)
            continue;
        if (memcmp(&event->classifyDrop->layerId, &connect4, sizeof(GUID)) != 0)
            continue;
        for (size_t k = 0; k < deny_count; k++) {
            if (event->classifyDrop->filterId == deny_ids[k]) {
                matched = deny_ids[k];
                break;
            }
        }
    }
    FwpmFreeMemory0((void **)&events);
    checked(FwpmNetEventDestroyEnumHandle0(engine, enum_handle),
            "net event enum destroy");
    return matched;
}

/* Diagnostic only: dump loopback listeners so the next gate failure names
 * what the exercise actually bound (IPv4 127.0.0.1 vs IPv6 ::1). Never
 * fails: diagnostics must not become a second gate. */
static void log_bound_addresses(void) {
    for (int v6 = 0; v6 <= 1; v6++) {
        ULONG family = v6 ? AF_INET6 : AF_INET;
        DWORD size = 0;
        GetExtendedTcpTable(NULL, &size, FALSE, family, TCP_TABLE_OWNER_PID_LISTENER, 0);
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER || size == 0) continue;
        BYTE *table = malloc(size);
        if (!table) continue;
        if (GetExtendedTcpTable(table, &size, FALSE, family, TCP_TABLE_OWNER_PID_LISTENER, 0)) {
            free(table);
            continue;
        }
        if (!v6) {
            MIB_TCPTABLE_OWNER_PID *tcp = (MIB_TCPTABLE_OWNER_PID *)table;
            for (DWORD i = 0; i < tcp->dwNumEntries; i++) {
                IN_ADDR addr;
                addr.S_un.S_addr = tcp->table[i].dwLocalAddr;
                char text[INET_ADDRSTRLEN] = {0};
                if (!InetNtopA(AF_INET, &addr, text, sizeof(text))) text[0] = 0;
                printf("WFP bound TCP %s:%u pid=%lu\n", text,
                       (unsigned)ntohs((u_short)tcp->table[i].dwLocalPort),
                       (unsigned long)tcp->table[i].dwOwningPid);
            }
        } else {
            MIB_TCP6TABLE_OWNER_PID *tcp = (MIB_TCP6TABLE_OWNER_PID *)table;
            for (DWORD i = 0; i < tcp->dwNumEntries; i++) {
                IN6_ADDR addr;
                char text[INET6_ADDRSTRLEN] = {0};
                memcpy(&addr, tcp->table[i].ucLocalAddr, sizeof(addr));
                if (!InetNtopA(AF_INET6, &addr, text, sizeof(text))) text[0] = 0;
                printf("WFP bound TCP6 %s:%u pid=%lu\n", text,
                       (unsigned)ntohs((u_short)tcp->table[i].dwLocalPort),
                       (unsigned long)tcp->table[i].dwOwningPid);
            }
        }
        free(table);
    }
    fflush(stdout);
}

/* One nonblocking TCP attempt with a bounded select fallback, classified
 * by wfp_classify_tcp_verdict. Returns the verdict; SO_ERROR and select
 * outputs are valid only for the WOULDBLOCK path the classifier reads. */
static int tcp_attempt(int family, UINT16 port, DWORD dest_v4, long deadline_sec,
                       int *sync_error, int *so_error) {
    SOCKET s = socket(family, SOCK_STREAM, 0);
    if (s == INVALID_SOCKET) {
        log_bound_addresses();
        checked(WSAGetLastError(), "control socket");
    }
    u_long nonblocking = 1;
    if (ioctlsocket(s, FIONBIO, &nonblocking)) {
        int status = WSAGetLastError();
        closesocket(s);
        log_bound_addresses();
        checked((DWORD)status, "nonblocking");
    }
    int rc;
    if (family == AF_INET) {
        struct sockaddr_in address = {0};
        address.sin_family = AF_INET;
        address.sin_port = htons(port);
        address.sin_addr.s_addr = htonl(dest_v4);
        rc = connect(s, (SOCKADDR *)&address, sizeof(address));
    } else {
        struct sockaddr_in6 address = {0};
        address.sin6_family = AF_INET6;
        address.sin6_port = htons(port);
        address.sin6_addr.s6_addr[15] = 1;
        rc = connect(s, (SOCKADDR *)&address, sizeof(address));
    }
    *sync_error = WSAGetLastError();
    int ready = 0;
    *so_error = *sync_error;
    if (rc == SOCKET_ERROR && *sync_error == WSAEWOULDBLOCK) {
        fd_set writes, errors;
        FD_ZERO(&writes); FD_ZERO(&errors);
        FD_SET(s, &writes); FD_SET(s, &errors);
        struct timeval timeout = {(long)deadline_sec, 0};
        ready = select(0, NULL, &writes, &errors, &timeout);
        if (ready > 0) {
            int size = sizeof(*so_error);
            if (getsockopt(s, SOL_SOCKET, SO_ERROR, (char *)so_error, &size)) {
                int status = WSAGetLastError();
                closesocket(s);
                log_bound_addresses();
                checked((DWORD)status, "control socket error");
            }
        }
    }
    closesocket(s);
    (void)rc;
    return wfp_classify_tcp_verdict(*sync_error, ready, *so_error);
}

static void denied(int family, int type, UINT16 port, BOOL external) {
    /* Fast loopback/IPv6 denies: no network is involved, so a matched ALE
     * block is synchronous WSAEACCES and anything else (including silence)
     * means no enforcement. Single attempt: silence here must fail loudly,
     * never retry into ambiguity. */
    (void)type;
    int sync_error = 0, so_error = 0;
    int verdict = tcp_attempt(family, port, external ? 0xc0000201 : 0x7f000001,
                              CONTROL_DEADLINE_SEC, &sync_error, &so_error);
    if (verdict != WFP_VERDICT_DENIED) {
        fprintf(stderr, "control did not get WSAEACCES: family=%d type=%d port=%u verdict=%d sync_error=%d so_error=%d\n",
                family, type, port, verdict, sync_error, so_error);
        log_bound_addresses();
        exit(1);
    }
    printf("WFP control denied family=%d type=%d port=%u WSAEACCES\n", family, type, port);
}

/* The external negative control (TEST-NET-1 TCP 443, never answers):
 *   - explicit ALE block -> WSAEACCES, the legacy proof line, PASS;
 *   - silence -> PASS only with a CLASSIFY_DROP net event proving OUR deny
 *     filter matched the flow (a slow-but-real verdict); silence with no
 *     such event is the run-38472050604 filter-miss/runner-drop case and
 *     fails with that diagnosis instead of a bare deadline;
 *   - connect/refuse/other -> no enforcement, fail.
 * Silence acceptance applies to THIS control only; the exercise stays
 * strict (any denial past the marker fails the gate script). */
static void denied_external(HANDLE engine, int family, UINT16 port) {
    FILETIME since;
    GetSystemTimeAsFileTime(&since);
    int sync_error = 0, so_error = 0;
    int verdict = tcp_attempt(family, port, 0xc0000201,
                              EXTERNAL_DEADLINE_SEC, &sync_error, &so_error);
    if (verdict == WFP_VERDICT_DENIED) {
        printf("WFP control denied family=%d type=%d port=%u WSAEACCES\n",
               family, SOCK_STREAM, port);
        return;
    }
    if (verdict == WFP_VERDICT_SILENT_DROP) {
        UINT64 matched = external_drop_matched(engine, &since);
        if (matched) {
            printf("WFP control silent-drop family=%d type=%d port=%u matched filter=%llu\n",
                   family, SOCK_STREAM, port, (unsigned long long)matched);
            return;
        }
        fprintf(stderr, "WFP gate failure: external control silent with no matching CLASSIFY_DROP event "
                "family=%d type=%d port=%u (filter missed the flow or the runner fabric dropped the SYN)\n",
                family, SOCK_STREAM, port);
        log_bound_addresses();
        exit(1);
    }
    fprintf(stderr, "control not denied: family=%d type=%d port=%u verdict=%d sync_error=%d so_error=%d\n",
            family, SOCK_STREAM, port, verdict, sync_error, so_error);
    log_bound_addresses();
    exit(1);
}

/* A successful UDP sendto only means Winsock accepted the bytes. Observe a
 * local receiver to prove whether WFP actually delivered the datagram. */
static void datagram_control(int family, UINT16 port, BOOL expect_delivery) {
    SOCKET receiver = socket(family, SOCK_DGRAM, IPPROTO_UDP);
    SOCKET sender = socket(family, SOCK_DGRAM, IPPROTO_UDP);
    if (receiver == INVALID_SOCKET || sender == INVALID_SOCKET)
        checked(WSAGetLastError(), "datagram control socket");
    struct sockaddr_storage address = {0};
    int length;
    if (family == AF_INET) {
        struct sockaddr_in *v4 = (struct sockaddr_in *)&address;
        v4->sin_family = AF_INET;
        v4->sin_port = htons(port);
        v4->sin_addr.s_addr = htonl(0x7f000001);
        length = sizeof(*v4);
    } else {
        struct sockaddr_in6 *v6 = (struct sockaddr_in6 *)&address;
        v6->sin6_family = AF_INET6;
        v6->sin6_port = htons(port);
        v6->sin6_addr.s6_addr[15] = 1;
        length = sizeof(*v6);
    }
    if (bind(receiver, (SOCKADDR *)&address, length)) checked(WSAGetLastError(), "datagram control bind");
    int sent = sendto(sender, "control", 7, 0, (SOCKADDR *)&address, length);
    if (sent != 7 && (sent != SOCKET_ERROR || WSAGetLastError() != WSAEACCES))
        checked(WSAGetLastError() ? WSAGetLastError() : 1, "datagram control send");
    fd_set reads;
    FD_ZERO(&reads);
    FD_SET(receiver, &reads);
    struct timeval timeout = {DATAGRAM_DEADLINE_SEC, 0};
    int ready = select(0, &reads, NULL, NULL, &timeout);
    if (ready == SOCKET_ERROR) {
        int status = WSAGetLastError();
        closesocket(sender);
        closesocket(receiver);
        log_bound_addresses();
        checked((DWORD)status, "datagram control receive");
    }
    if ((ready == 1) != expect_delivery) {
        closesocket(sender);
        closesocket(receiver);
        log_bound_addresses();
        checked(1, "datagram delivery verdict");
    }
    if (ready == 1) {
        char received[8];
        if (recvfrom(receiver, received, sizeof(received), 0, NULL, NULL) != 7 ||
            memcmp(received, "control", 7) != 0) {
            closesocket(sender);
            closesocket(receiver);
            log_bound_addresses();
            checked(1, "datagram content verdict");
        }
    }
    closesocket(sender);
    closesocket(receiver);
    printf("WFP UDP control %s family=%d port=%u\n", expect_delivery ? "delivered" : "denied", family, port);
}

static void allowed(UINT16 port) {
    SOCKET listener = socket(AF_INET, SOCK_STREAM, 0), client = socket(AF_INET, SOCK_STREAM, 0);
    if (listener == INVALID_SOCKET || client == INVALID_SOCKET) checked(WSAGetLastError(), "allow socket");
    BOOL exclusive = TRUE;
    if (setsockopt(listener, SOL_SOCKET, SO_EXCLUSIVEADDRUSE, (char *)&exclusive, sizeof(exclusive)))
        checked(WSAGetLastError(), "exclusive bind");
    struct sockaddr_in address = {0};
    address.sin_family = AF_INET;
    address.sin_port = htons(port);
    address.sin_addr.s_addr = htonl(0x7f000001);
    if (bind(listener, (SOCKADDR *)&address, sizeof(address)) || listen(listener, 1) ||
        connect(client, (SOCKADDR *)&address, sizeof(address))) {
        int status = WSAGetLastError();
        closesocket(client);
        closesocket(listener);
        log_bound_addresses();
        checked((DWORD)status, "allowed loopback");
    }
    SOCKET accepted = accept(listener, NULL, NULL);
    if (accepted == INVALID_SOCKET) {
        int status = WSAGetLastError();
        closesocket(client);
        closesocket(listener);
        log_bound_addresses();
        checked((DWORD)status, "accept control");
    }
    shutdown(client, SD_SEND);
    char byte;
    if (recv(accepted, &byte, 1, 0) != 0) checked(1, "control EOF");
    closesocket(accepted);
    closesocket(client);
    closesocket(listener);
    puts("WFP configured loopback control allowed");
}

/* CommandLineToArgvW-compatible quoting, including embedded/trailing slashes. */
static wchar_t *command_line(int argc, wchar_t **argv) {
    size_t capacity = 1;
    for (int i = 0; i < argc; i++) capacity += 2 * wcslen(argv[i]) + 4;
    wchar_t *result = calloc(capacity, sizeof(wchar_t)), *p = result;
    if (!result) checked(ERROR_NOT_ENOUGH_MEMORY, "command line");
    for (int i = 0; i < argc; i++) {
        *p++ = L'"';
        const wchar_t *s = argv[i];
        while (*s) {
            size_t slashes = 0;
            while (*s == L'\\') { slashes++; s++; }
            size_t count = (*s == L'"' || !*s) ? 2 * slashes : slashes;
            while (count--) *p++ = L'\\';
            if (*s == L'"') *p++ = L'\\';
            if (*s) *p++ = *s++;
        }
        *p++ = L'"'; *p++ = L' ';
    }
    *p = 0;
    return result;
}

int wmain(int argc, wchar_t **argv) {
    /* guard.exe --app EXE [--app EXE ...] -- PIDFILE EXECUTABLE [ARGS...] */
    int dashdash = -1;
    for (int i = 1; i < argc; i++) {
        if (!wcscmp(argv[i], L"--")) { dashdash = i; break; }
        if (!wcscmp(argv[i], L"--app")) i++;
    }
    if (dashdash < 0 || argc - dashdash < 3) return 2;
    WSADATA data;
    checked((DWORD)WSAStartup(MAKEWORD(2, 2), &data), "WSAStartup");
    HANDLE engine = NULL;
    FWPM_SESSION0 session = {0};
    session.flags = 1; /* FWPM_SESSION_FLAG_DYNAMIC */
    checked(FwpmEngineOpen0(NULL, RPC_C_AUTHN_WINNT, NULL, &session, &engine), "engine open (admin required)");
    FWPM_SUBLAYER0 sublayer = {0};
    checked(UuidCreate(&sublayer.subLayerKey), "unique sublayer");
    sublayer.displayData.name = L"Bashy disposable prima gate";
    sublayer.weight = 0xffff;
    checked(FwpmSubLayerAdd0(engine, &sublayer, NULL), "sublayer add");
    wchar_t self[32768];
    DWORD length = GetModuleFileNameW(NULL, self, 32768);
    wincheck(length && length < 32768, "self path");
    protect(engine, sublayer.subLayerKey, self);
    verify_deny_filters_installed(engine, &sublayer.subLayerKey);
    /* Fast denies first: their WSAEACCES proves app-id matching for this
     * process before the external verdict is read. The external control
     * runs last so its verdict (or proven silence) cannot be mistaken for
     * a cold-start delay in the fast path. */
    denied(AF_INET, SOCK_STREAM, 5606, FALSE);
    denied(AF_INET6, SOCK_STREAM, 5599, FALSE);
    denied_external(engine, AF_INET, 443);
    datagram_control(AF_INET, 5599, TRUE);
    datagram_control(AF_INET, 5606, FALSE);
    datagram_control(AF_INET6, 5599, FALSE);
    allowed(5599);
    /* Resolve the spawned executable to a full path for app-id filtering. */
    wchar_t full[32768];
    wchar_t *rest = NULL;
    DWORD got = SearchPathW(NULL, argv[dashdash + 2], L".exe", 32768, full, &rest);
    wincheck(got && got < 32768, "resolve exercise executable");
    argv[dashdash + 2] = full;
    protect(engine, sublayer.subLayerKey, full);
    for (int i = 1; i < dashdash; i++) {
        if (!wcscmp(argv[i], L"--app") && i + 1 < dashdash)
            protect(engine, sublayer.subLayerKey, argv[i + 1]);
    }
    /* allowed_ports documents the allowlist the permit and block range tiles
     * cover; check_test.py verifies both tilings against this same list. */
    (void)allowed_ports;
    HANDLE job = CreateJobObjectW(NULL, NULL);
    wincheck(job != NULL, "create job");
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits = {0};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    wincheck(SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits)), "job limits");
    STARTUPINFOW startup = {0};
    startup.cb = sizeof(startup);
    startup.dwFlags = STARTF_USESTDHANDLES;
    startup.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
    startup.hStdOutput = GetStdHandle(STD_OUTPUT_HANDLE);
    startup.hStdError = GetStdHandle(STD_ERROR_HANDLE);
    PROCESS_INFORMATION process = {0};
    wchar_t *command = command_line(argc - dashdash - 2, argv + dashdash + 2);
    wincheck(CreateProcessW(full, command, NULL, NULL, TRUE, CREATE_SUSPENDED | CREATE_NO_WINDOW,
                            NULL, NULL, &startup, &process), "create suspended exercise");
    free(command);
    if (!AssignProcessToJobObject(job, process.hProcess)) {
        TerminateProcess(process.hProcess, 1);
        WaitForSingleObject(process.hProcess, 5000);
        checked(1, "assign job");
    }
    FILE *pidfile = _wfopen(argv[dashdash + 1], L"w");
    if (!pidfile) checked(1, "pid file");
    fprintf(pidfile, "%lu\n", (unsigned long)process.dwProcessId);
    if (fclose(pidfile)) checked(1, "pid file close");
    if (ResumeThread(process.hThread) == (DWORD)-1) checked(GetLastError(), "resume exercise");
    CloseHandle(process.hThread);
    puts("WFP exercise filters installed; multi-process job active");
    fflush(stdout);
    /* The exercise owns per-stage deadlines; the guard owns the outer one. */
    DWORD wait = WaitForSingleObject(process.hProcess, 600000);
    DWORD code = 1;
    if (wait != WAIT_OBJECT_0) {
        fprintf(stderr, "exercise exceeded the outer deadline\n");
        /* Hung ranks are still bound here: name IPv4 loopback vs IPv6
         * before the job kill reaps them. */
        log_bound_addresses();
        TerminateJobObject(job, 1);
        wincheck(WaitForSingleObject(process.hProcess, 5000) == WAIT_OBJECT_0, "wait exercise");
    }
    wincheck(GetExitCodeProcess(process.hProcess, &code), "exit status");
    if (code != 0) log_bound_addresses();
    CloseHandle(process.hProcess);
    CloseHandle(job);
    checked(FwpmEngineClose0(engine), "remove dynamic policy");
    WSACleanup();
    puts("exercise reaped; job closed; dynamic WFP policy removed");
    return code == 0 ? 0 : 1;
}
