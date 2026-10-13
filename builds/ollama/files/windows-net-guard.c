/* Hosted Windows only. Dynamic WFP policy exists before CreateProcess and until
 * after WaitForSingleObject. A one-process job prevents unfiltered descendants.
 * WFP ALE_AUTH_CONNECT covers TCP, UDP and non-error ICMP, including loopback.
 * GUIDs: Microsoft Windows SDK fwpmu.h; constants named locally because the
 * hosted MinGW header does not expose every SDK layer/condition GUID.
 */
#define _WIN32_WINNT 0x0602
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <fwpmu.h>
#include <rpc.h>
#include <stdio.h>
#include <stdlib.h>
#include <wchar.h>

static const GUID connect4 = {0xc38d57d1,0x05a7,0x4c33,{0x90,0x4f,0x7f,0xbc,0xee,0xe6,0x0e,0x82}};
static const GUID connect6 = {0x4a72393b,0x319f,0x44bc,{0x84,0xc3,0xba,0x54,0xdc,0xb3,0xb6,0xb4}};
static const GUID app_id = {0xd78e1e87,0x8644,0x4ea5,{0x94,0x37,0xd8,0x09,0xec,0xef,0xc9,0x71}};
static const GUID remote_address = {0xb235ae9a,0x1d64,0x49b8,{0xa4,0x4c,0x5f,0xf3,0xd9,0x09,0x50,0x45}};
static const GUID remote_port = {0xc35a604d,0xd22b,0x4e1a,{0x91,0xb4,0x68,0xf6,0x74,0xee,0x67,0x4b}};

static void checked(DWORD status, const char *operation) {
    if (status) {
        fprintf(stderr, "WFP gate failure: %s: %lu\n", operation, (unsigned long)status);
        exit(1);
    }
}
static void wincheck(BOOL ok, const char *operation) {
    if (!ok) checked(GetLastError() ? GetLastError() : 1, operation);
}

static void protect(HANDLE engine, GUID sublayer, const wchar_t *exe, UINT16 port) {
    FWP_BYTE_BLOB *app = NULL;
    checked(FwpmGetAppIdFromFileName0(exe, &app), "app identity");
    FWPM_FILTER_CONDITION0 conditions[2] = {0};
    conditions[0].fieldKey = app_id;
    conditions[0].matchType = FWP_MATCH_EQUAL;
    conditions[0].conditionValue.type = FWP_BYTE_BLOB_TYPE;
    conditions[0].conditionValue.byteBlob = app;
    FWPM_FILTER0 filter = {0};
    filter.displayData.name = L"Bashy hosted runner no-egress";
    filter.subLayerKey = sublayer;
    filter.action.type = FWP_ACTION_BLOCK;
    filter.weight.type = FWP_EMPTY;
    filter.filterCondition = conditions;
    /* IPv6 completely denied: the sole configured endpoint is IPv4. */
    filter.layerKey = connect6;
    filter.numFilterConditions = 1;
    checked(FwpmFilterAdd0(engine, &filter, NULL, NULL), "IPv6 deny");
    filter.layerKey = connect4;
    filter.numFilterConditions = 2;
    conditions[1].fieldKey = remote_address;
    conditions[1].matchType = FWP_MATCH_NOT_EQUAL;
    conditions[1].conditionValue.type = FWP_UINT32;
    conditions[1].conditionValue.uint32 = 0x7f000001; /* WFP host byte order */
    checked(FwpmFilterAdd0(engine, &filter, NULL, NULL), "non-loopback deny");
    conditions[1].fieldKey = remote_port;
    conditions[1].conditionValue.type = FWP_UINT16;
    conditions[1].conditionValue.uint16 = port;
    checked(FwpmFilterAdd0(engine, &filter, NULL, NULL), "other ports deny");
    FwpmFreeMemory0((void **)&app);
}

static void denied(int family, int type, UINT16 port, BOOL external) {
    SOCKET s = socket(family, type, 0);
    if (s == INVALID_SOCKET) checked(WSAGetLastError(), "control socket");
    /* Missing enforcement must not wait for an external route timeout. */
    u_long nonblocking = 1;
    if (ioctlsocket(s, FIONBIO, &nonblocking)) checked(WSAGetLastError(), "nonblocking");
    int rc;
    if (family == AF_INET) {
        struct sockaddr_in address = {0};
        address.sin_family = AF_INET;
        address.sin_port = htons(port);
        address.sin_addr.s_addr = htonl(external ? 0xc0000201 : 0x7f000001);
        rc = type == SOCK_DGRAM ? sendto(s, "control", 7, 0, (SOCKADDR *)&address, sizeof(address)) :
                                 connect(s, (SOCKADDR *)&address, sizeof(address));
    } else {
        struct sockaddr_in6 address = {0};
        address.sin6_family = AF_INET6;
        address.sin6_port = htons(port);
        address.sin6_addr.s6_addr[15] = 1;
        rc = type == SOCK_DGRAM ? sendto(s, "control", 7, 0, (SOCKADDR *)&address, sizeof(address)) :
                                 connect(s, (SOCKADDR *)&address, sizeof(address));
    }
    int error = WSAGetLastError();
    if (type == SOCK_STREAM && rc == SOCKET_ERROR && error == WSAEWOULDBLOCK) {
        fd_set writes, errors;
        FD_ZERO(&writes); FD_ZERO(&errors);
        FD_SET(s, &writes); FD_SET(s, &errors);
        struct timeval timeout = {2, 0};
        if (select(0, NULL, &writes, &errors, &timeout) <= 0) checked(1, "control connect deadline");
        int size = sizeof(error);
        if (getsockopt(s, SOL_SOCKET, SO_ERROR, (char *)&error, &size)) checked(WSAGetLastError(), "control socket error");
    }
    closesocket(s);
    if (rc != SOCKET_ERROR || error != WSAEACCES) {
        fprintf(stderr, "control did not get WSAEACCES: family=%d type=%d port=%u rc=%d error=%d\n",
                family, type, port, rc, error);
        exit(1);
    }
    printf("WFP control denied family=%d type=%d port=%u WSAEACCES\n", family, type, port);
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
        connect(client, (SOCKADDR *)&address, sizeof(address))) checked(WSAGetLastError(), "allowed loopback");
    SOCKET accepted = accept(listener, NULL, NULL);
    if (accepted == INVALID_SOCKET) checked(WSAGetLastError(), "accept control");
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
    if (argc < 4) return 2; /* PORT PIDFILE EXECUTABLE [ARGS...] */
    unsigned long value = wcstoul(argv[1], NULL, 10);
    if (value < 1024 || value > 65534) return 2;
    UINT16 port = (UINT16)value;
    WSADATA data;
    checked((DWORD)WSAStartup(MAKEWORD(2, 2), &data), "WSAStartup");
    HANDLE engine = NULL;
    FWPM_SESSION0 session = {0};
    session.flags = 1; /* FWPM_SESSION_FLAG_DYNAMIC */
    checked(FwpmEngineOpen0(NULL, RPC_C_AUTHN_WINNT, NULL, &session, &engine), "engine open (admin required)");
    FWPM_SUBLAYER0 sublayer = {0};
    checked(UuidCreate(&sublayer.subLayerKey), "unique sublayer");
    sublayer.displayData.name = L"Bashy disposable native gate";
    sublayer.weight = 0xffff;
    checked(FwpmSubLayerAdd0(engine, &sublayer, NULL), "sublayer add");
    wchar_t self[32768];
    DWORD length = GetModuleFileNameW(NULL, self, 32768);
    wincheck(length && length < 32768, "self path");
    protect(engine, sublayer.subLayerKey, self, port);
    for (int i = 0; i < 2; i++) {
        int type = i ? SOCK_DGRAM : SOCK_STREAM;
        denied(AF_INET, type, 443, TRUE);
        denied(AF_INET, type, (UINT16)(port + 1), FALSE);
        denied(AF_INET6, type, port, FALSE);
    }
    allowed(port);
    protect(engine, sublayer.subLayerKey, argv[3], port);
    HANDLE job = CreateJobObjectW(NULL, NULL);
    wincheck(job != NULL, "create job");
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits = {0};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | JOB_OBJECT_LIMIT_ACTIVE_PROCESS;
    limits.BasicLimitInformation.ActiveProcessLimit = 1;
    wincheck(SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits)), "job limits");
    STARTUPINFOW startup = {0};
    startup.cb = sizeof(startup);
    startup.dwFlags = STARTF_USESTDHANDLES;
    startup.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
    startup.hStdOutput = GetStdHandle(STD_OUTPUT_HANDLE);
    startup.hStdError = GetStdHandle(STD_ERROR_HANDLE);
    PROCESS_INFORMATION process = {0};
    wchar_t *command = command_line(argc - 3, argv + 3);
    wincheck(CreateProcessW(argv[3], command, NULL, NULL, TRUE, CREATE_SUSPENDED | CREATE_NO_WINDOW,
                            NULL, NULL, &startup, &process), "create suspended runner");
    free(command);
    if (!AssignProcessToJobObject(job, process.hProcess)) {
        TerminateProcess(process.hProcess, 1);
        WaitForSingleObject(process.hProcess, 5000);
        checked(1, "assign job");
    }
    FILE *pidfile = _wfopen(argv[2], L"w");
    if (!pidfile) checked(1, "pid file");
    fprintf(pidfile, "%lu\n", (unsigned long)process.dwProcessId);
    if (fclose(pidfile)) checked(1, "pid file close");
    if (ResumeThread(process.hThread) == (DWORD)-1) checked(GetLastError(), "resume runner");
    CloseHandle(process.hThread);
    puts("WFP runner filters installed; single-process job active");
    fflush(stdout);
    /* Poll pipe so spontaneous runner failure exits promptly; no inherited stdin
     * EOF dependency. Parent owns the deadline and requests explicit shutdown. */
    BOOL stopping = FALSE;
    while (WaitForSingleObject(process.hProcess, 50) == WAIT_TIMEOUT) {
        DWORD available = 0;
        if (!PeekNamedPipe(GetStdHandle(STD_INPUT_HANDLE), NULL, 0, NULL, &available, NULL)) break;
        if (available) { stopping = TRUE; break; }
    }
    DWORD code = 1;
    if (stopping) wincheck(TerminateJobObject(job, 0), "terminate job");
    else TerminateJobObject(job, 1);
    wincheck(WaitForSingleObject(process.hProcess, 5000) == WAIT_OBJECT_0, "wait runner");
    wincheck(GetExitCodeProcess(process.hProcess, &code), "exit status");
    CloseHandle(process.hProcess);
    CloseHandle(job);
    checked(FwpmEngineClose0(engine), "remove dynamic policy");
    WSACleanup();
    puts("runner reaped; job closed; dynamic WFP policy removed");
    return stopping && code == 0 ? 0 : 1;
}
