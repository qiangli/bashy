// zmq_nng_shim.hpp
// ---------------------------------------------------------------------------
// A small `namespace zmq` compatibility shim that re-implements EXACTLY the
// subset of the cppzmq / zmq_addon.hpp API that prima.cpp uses, backed by
// MIT-licensed nng (libnng) instead of MPL-2.0 ZeroMQ.
//
// Goal: prima.cpp's existing `zmq::` call sites compile and run UNCHANGED.
// This file is intended to replace `#include "zmq_addon.hpp"` in
// src/llama.cpp (STEP 2 — not done yet).
//
// Design notes / assumptions (see report):
//   * prima uses ONLY PUSH/PULL (pipeline0) sockets. PULL sockets bind(),
//     PUSH sockets connect(). nng_push0_open / nng_pull0_open mirror this 1:1.
//   * ZeroMQ multipart is emulated over ONE nng message using the frame
//     format  [uint32 n_parts][ (uint32 part_len)(part_bytes) ]*n_parts .
//     Both peers run this same shim, so the encoding is symmetric. Integers
//     are written in host byte order (prima's fleet is all little-endian
//     x86/arm); this matches prima's own raw-struct memcpy wire usage.
//   * cppzmq's send_multipart returns optional<size_t> (#parts) and
//     recv_multipart returns optional<size_t> (#parts, empty on timeout).
//     prima only checks `if (!recv_multipart(...))`, so empty-optional on
//     NNG_ETIMEDOUT reproduces ZeroMQ's RCVTIMEO behaviour.
//   * sockopt::rcvtimeo  -> NNG_OPT_RECVTIMEO (value passthrough; -1 == block
//     forever == NNG_DURATION_INFINITE, same numeric value).
//     sockopt::linger / sockopt::rcvmore have no nng equivalent and are kept
//     as compile-only stand-ins (linger stored & ignored; rcvmore handled via
//     the single-message recv path below).
// ---------------------------------------------------------------------------
#pragma once

#include <nng/nng.h>
#include <nng/protocol/pipeline0/push.h>
#include <nng/protocol/pipeline0/pull.h>

#include <cstdint>
#include <cstring>
#include <exception>
#include <iterator>
#include <optional>
#include <string>
#include <vector>

// Legacy ZMQ_* option macros referenced by prima via setsockopt/getsockopt.
#ifndef ZMQ_RCVTIMEO
#define ZMQ_RCVTIMEO 27
#endif
#ifndef ZMQ_RCVMORE
#define ZMQ_RCVMORE 13
#endif
#ifndef ZMQ_LINGER
#define ZMQ_LINGER 17
#endif

namespace zmq {

// ---------------------------------------------------------------------------
// optional<T> compatibility.
//   cppzmq's send_multipart / recv_multipart return optional<size_t>
//   (and recv()/send() return optional<size_t>) under C++17. prima.cpp's
//   GNU Makefile builds at -std=c++11, where std::optional does not exist, so
//   provide a minimal bool-convertible fallback. Every prima call site only
//   tests the result for truthiness (`if (!recv_multipart(...))`,
//   `if (!recv_result)`), so the fallback is behaviourally exact. Under C++17
//   the real std::optional is used, preserving the standalone round-trip test.
// ---------------------------------------------------------------------------
#if defined(__cpp_lib_optional) || (defined(__cplusplus) && __cplusplus >= 201703L)
template <typename T> using optional = std::optional<T>;
inline constexpr std::nullopt_t nullopt = std::nullopt;
#else
struct nullopt_t {};
constexpr nullopt_t nullopt{};
template <typename T>
class optional {
public:
    optional() : has_(false), val_() {}
    optional(nullopt_t) : has_(false), val_() {}
    optional(T v) : has_(true), val_(v) {}
    explicit operator bool() const { return has_; }
    bool   operator!() const { return !has_; }
    const T & operator*() const { return val_; }
    T      value() const { return val_; }
private:
    bool has_;
    T    val_;
};
#endif

// ---------------------------------------------------------------------------
// error_t — std::exception carrying an nng error string.
// ---------------------------------------------------------------------------
class error_t : public std::exception {
public:
    error_t() : code_(0), msg_("unknown zmq/nng error") {}
    explicit error_t(int code) : code_(code), msg_(nng_strerror(code)) {}
    error_t(int code, const std::string & where)
        : code_(code), msg_(where + ": " + nng_strerror(code)) {}
    const char * what() const noexcept override { return msg_.c_str(); }
    int num() const noexcept { return code_; }

private:
    int         code_;
    std::string msg_;
};

// ---------------------------------------------------------------------------
// context_t — nng has no context object; this is a trivial holder so that
// `zmq::context_t(2)` and `socket_t(ctx, ...)` compile.
// ---------------------------------------------------------------------------
class context_t {
public:
    context_t() = default;
    explicit context_t(int /*io_threads*/) {}
    ~context_t() = default;
};

// ---------------------------------------------------------------------------
// socket_type / flags / sockopt tags.
// ---------------------------------------------------------------------------
enum class socket_type { push, pull };

enum class send_flags : int { none = 0, dontwait = 1, sndmore = 2 };
enum class recv_flags : int { none = 0, dontwait = 1 };

namespace sockopt {
struct rcvtimeo_t {};
struct sndtimeo_t {};
struct linger_t   {};
struct rcvmore_t  {};
inline constexpr rcvtimeo_t rcvtimeo{};
inline constexpr sndtimeo_t sndtimeo{};
inline constexpr linger_t   linger{};
inline constexpr rcvmore_t  rcvmore{};
} // namespace sockopt

// ---------------------------------------------------------------------------
// message_t — owns a byte buffer. Move-only, matching cppzmq.
// ---------------------------------------------------------------------------
class message_t {
public:
    message_t() = default;
    explicit message_t(size_t size) : buf_(size) {}
    message_t(const void * data, size_t size)
        : buf_(static_cast<const uint8_t *>(data),
               static_cast<const uint8_t *>(data) + size) {}

    // move-only
    message_t(const message_t &)             = delete;
    message_t & operator=(const message_t &) = delete;
    message_t(message_t &&) noexcept            = default;
    message_t & operator=(message_t &&) noexcept = default;

    void *       data()       noexcept { return buf_.empty() ? nullptr : buf_.data(); }
    const void * data() const noexcept { return buf_.empty() ? nullptr : buf_.data(); }
    size_t       size() const noexcept { return buf_.size(); }

    std::string to_string() const {
        return std::string(reinterpret_cast<const char *>(buf_.data()), buf_.size());
    }

private:
    std::vector<uint8_t> buf_;
};

// ---------------------------------------------------------------------------
// socket_t.
// ---------------------------------------------------------------------------
class socket_t {
public:
    socket_t() = default;

    socket_t(context_t & /*ctx*/, socket_type type) {
        int rv = 0;
        switch (type) {
            case socket_type::push: rv = nng_push0_open(&sock_); break;
            case socket_type::pull: rv = nng_pull0_open(&sock_); break;
        }
        if (rv != 0) throw error_t(rv, "nng_open");
        open_ = true;
    }

    // move-only (sockets are not copyable)
    socket_t(const socket_t &)             = delete;
    socket_t & operator=(const socket_t &) = delete;
    socket_t(socket_t && o) noexcept { swap(o); }
    socket_t & operator=(socket_t && o) noexcept { if (this != &o) { do_close(); swap(o); } return *this; }

    ~socket_t() { do_close(); }

    // PULL endpoints bind; PUSH endpoints connect.
    void bind(const std::string & endpoint) {
        const std::string url = to_nng_url(endpoint);
        int rv = nng_listen(sock_, url.c_str(), nullptr, 0);
        if (rv != 0) throw error_t(rv, "nng_listen(" + url + ")");
    }

    void connect(const std::string & endpoint) {
        const std::string url = to_nng_url(endpoint);
        // ZeroMQ's connect() is ASYNCHRONOUS and LAZY: it never fails at call
        // time — it records the endpoint and (re)connects in the background,
        // retrying forever until a valid peer appears. prima.cpp relies on this:
        // llama_init_sockets() brings every rank of the pipelined ring up at
        // ~the same instant (each binds its own recv PULL, then immediately
        // dials the peers' recv PULLs), so a connect() MUST tolerate the peer
        // not having bound yet.
        //
        // nng's default nng_dial (flags == 0) is SYNCHRONOUS and fails hard:
        //   * NNG_ECONNREFUSED — peer's listener isn't up yet (the common race);
        //   * NNG_EPROTO       — it briefly connected to a port still held by a
        //                        stale/half-open endpoint from a previous
        //                        crashed run, whose bytes aren't a valid nng SP
        //                        header (tcptran_pipe_nego_cb magic check);
        //   * NNG_ECONNSHUT    — peer closed mid-negotiation.
        // Any of these aborts llama_init_sockets() (exit(1)) even though the
        // socket orientation is correct (PULL binds, PUSH connects).
        //
        // NNG_FLAG_NONBLOCK makes nng_dial return immediately and the dialer
        // retry with backoff in the background until the peer is ready — i.e.
        // exactly ZeroMQ connect() semantics. With this flag nng_dial only
        // reports synchronous *setup* errors (bad URL / ENOMEM), never the
        // transient connection failures above.
        int rv = nng_dial(sock_, url.c_str(), nullptr, NNG_FLAG_NONBLOCK);
        if (rv != 0) throw error_t(rv, "nng_dial(" + url + ")");
    }

    // --- modern cppzmq option setters -------------------------------------
    void set(sockopt::rcvtimeo_t, int ms) {
        // cppzmq: -1 == infinite; nng NNG_DURATION_INFINITE is also -1.
        int rv = nng_socket_set_ms(sock_, NNG_OPT_RECVTIMEO, (nng_duration) ms);
        if (rv != 0) throw error_t(rv, "set rcvtimeo");
    }
    void set(sockopt::sndtimeo_t, int ms) {
        int rv = nng_socket_set_ms(sock_, NNG_OPT_SENDTIMEO, (nng_duration) ms);
        if (rv != 0) throw error_t(rv, "set sndtimeo");
    }
    void set(sockopt::linger_t, int ms) {
        // nng has no socket-level linger equivalent; keep for compatibility.
        linger_ms_ = ms;
    }

    // --- legacy setsockopt/getsockopt -------------------------------------
    void setsockopt(int option, const void * optval, size_t optvallen) {
        if (option == ZMQ_RCVTIMEO && optvallen >= sizeof(int)) {
            int ms = 0;
            std::memcpy(&ms, optval, sizeof(int));
            set(sockopt::rcvtimeo, ms);
        } else if (option == ZMQ_LINGER && optvallen >= sizeof(int)) {
            std::memcpy(&linger_ms_, optval, sizeof(int));
        }
        // other options: silently ignored (none used by prima)
    }
    void getsockopt(int option, void * optval, size_t * optvallen) {
        if (option == ZMQ_RCVMORE && optval && optvallen && *optvallen >= sizeof(int)) {
            int more = more_flag_;
            std::memcpy(optval, &more, sizeof(int));
            *optvallen = sizeof(int);
        }
    }

    // --- single-message send/recv (cppzmq returns optional<size_t>) ---
    // const ref binds both lvalues and temporaries (e.g. send(message_t("STOP",4)));
    // bytes are copied into the nng msg, so no mutation of msg is needed.
    optional<size_t> send(const message_t & msg, send_flags flags) {
        nng_msg * m = nullptr;
        int rv = nng_msg_alloc(&m, 0);
        if (rv != 0) throw error_t(rv, "nng_msg_alloc");
        if (msg.size() > 0) {
            rv = nng_msg_append(m, msg.data(), msg.size());
            if (rv != 0) { nng_msg_free(m); throw error_t(rv, "nng_msg_append"); }
        }
        int nflags = (static_cast<int>(flags) & static_cast<int>(send_flags::dontwait))
                         ? NNG_FLAG_NONBLOCK : 0;
        rv = nng_sendmsg(sock_, m, nflags);
        if (rv != 0) {
            nng_msg_free(m);
            if (rv == NNG_ETIMEDOUT || rv == NNG_EAGAIN) return nullopt;
            throw error_t(rv, "nng_sendmsg");
        }
        return msg.size(); // nng took ownership of m
    }

    optional<size_t> recv(message_t & out, recv_flags flags) {
        nng_msg * m = nullptr;
        int nflags = (static_cast<int>(flags) & static_cast<int>(recv_flags::dontwait))
                         ? NNG_FLAG_NONBLOCK : 0;
        int rv = nng_recvmsg(sock_, &m, nflags);
        if (rv != 0) {
            if (rv == NNG_ETIMEDOUT || rv == NNG_EAGAIN) return nullopt;
            throw error_t(rv, "nng_recvmsg");
        }
        size_t len = nng_msg_len(m);
        out = message_t(nng_msg_body(m), len);
        nng_msg_free(m);
        more_flag_ = 0; // single-message model: never "more"
        return len;
    }

    void close() { do_close(); }

    // raw nng handle (used by the multipart free functions below)
    nng_socket  raw()    const { return sock_; }
    bool        is_open() const { return open_; }

private:
    void do_close() {
        if (open_) {
            nng_close(sock_);
            open_ = false;
        }
    }
    void swap(socket_t & o) noexcept {
        std::swap(sock_, o.sock_);
        std::swap(open_, o.open_);
        std::swap(linger_ms_, o.linger_ms_);
        std::swap(more_flag_, o.more_flag_);
    }

    // Map a ZeroMQ "tcp://host:port" endpoint to the nng equivalent. nng uses
    // the same scheme; the only quirk is the ZeroMQ wildcard bind address
    // "tcp://*:port", which nng expresses as "tcp://0.0.0.0:port".
    static std::string to_nng_url(const std::string & ep) {
        const std::string pfx = "tcp://*:";
        if (ep.compare(0, pfx.size(), pfx) == 0) {
            return "tcp://0.0.0.0:" + ep.substr(pfx.size());
        }
        return ep;
    }

    nng_socket sock_      = NNG_SOCKET_INITIALIZER;
    bool       open_      = false;
    int        linger_ms_ = 0;
    int        more_flag_ = 0;
};

// ---------------------------------------------------------------------------
// Multipart emulation over a single nng message.
//   frame = [uint32 n_parts] then for each part: [uint32 len][len bytes]
// ---------------------------------------------------------------------------
namespace detail {

inline void put_u32(nng_msg * m, uint32_t v) {
    // host byte order (symmetric: both peers run this shim)
    if (nng_msg_append(m, &v, sizeof(v)) != 0) throw error_t(NNG_ENOMEM, "append u32");
}

inline bool get_u32(const uint8_t *& p, const uint8_t * end, uint32_t & out) {
    if (static_cast<size_t>(end - p) < sizeof(uint32_t)) return false;
    std::memcpy(&out, p, sizeof(uint32_t));
    p += sizeof(uint32_t);
    return true;
}

} // namespace detail

// cppzmq: optional<size_t> send_multipart(socket, range&)
inline optional<size_t>
send_multipart(socket_t & socket, std::vector<message_t> & msgs) {
    nng_msg * m = nullptr;
    int rv = nng_msg_alloc(&m, 0);
    if (rv != 0) throw error_t(rv, "nng_msg_alloc");

    detail::put_u32(m, static_cast<uint32_t>(msgs.size()));
    for (auto & part : msgs) {
        detail::put_u32(m, static_cast<uint32_t>(part.size()));
        if (part.size() > 0) {
            rv = nng_msg_append(m, part.data(), part.size());
            if (rv != 0) { nng_msg_free(m); throw error_t(rv, "nng_msg_append"); }
        }
    }

    rv = nng_sendmsg(socket.raw(), m, 0);
    if (rv != 0) { nng_msg_free(m); throw error_t(rv, "nng_sendmsg"); }
    return msgs.size();
}

// cppzmq: template<OutputIt> optional<size_t> recv_multipart(socket, OutputIt)
template <typename OutputIt>
optional<size_t> recv_multipart(socket_t & socket, OutputIt out) {
    nng_msg * m = nullptr;
    int rv = nng_recvmsg(socket.raw(), &m, 0);
    if (rv != 0) {
        if (rv == NNG_ETIMEDOUT || rv == NNG_EAGAIN) return nullopt;
        throw error_t(rv, "nng_recvmsg");
    }

    const uint8_t * p   = static_cast<const uint8_t *>(nng_msg_body(m));
    const uint8_t * end = p + nng_msg_len(m);

    uint32_t n_parts = 0;
    if (!detail::get_u32(p, end, n_parts)) {
        nng_msg_free(m);
        throw error_t(NNG_EINVAL, "recv_multipart: truncated header");
    }

    size_t count = 0;
    for (uint32_t i = 0; i < n_parts; ++i) {
        uint32_t len = 0;
        if (!detail::get_u32(p, end, len) ||
            static_cast<size_t>(end - p) < len) {
            nng_msg_free(m);
            throw error_t(NNG_EINVAL, "recv_multipart: truncated part");
        }
        *out++ = message_t(p, len);
        p += len;
        ++count;
    }

    nng_msg_free(m);
    return count;
}

} // namespace zmq
