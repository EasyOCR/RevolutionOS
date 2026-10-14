/*
 * LD_PRELOAD shim: transparently remaps bind()/connect() calls targeting
 * 127.0.0.1:55142 to a per-instance private port instead.
 *
 * Why this exists: Sober's own single-instance self-check does a
 * bind-if-primary-else-connect-to-primary handshake on 127.0.0.1:55142.
 * Confirmed via a live bpftrace syscall capture. All orchestrator instances
 * share one network namespace (flatpak's manifest grants shared=network),
 * so a second instance's connect() to that port lands directly in a FIRST
 * instance's real listener instead of failing cleanly - that cross-instance
 * reachability is what the primary instance's code misreads as a "frozen"
 * sibling. A full per-instance network namespace would also fix this, but
 * needs real NAT/DNS/routing infrastructure for every other port too; this
 * shim only touches the one problem port and leaves every other syscall,
 * including all of Sober's actual internet traffic, completely untouched.
 *
 * REVOS_PORT_REMAP=<port> picks the per-instance replacement port (set by
 * the orchestrator at launch time, derived from the instance's allocated
 * display number so it's unique per instance). Without that env var set,
 * this shim is a no-op passthrough.
 */
#define _GNU_SOURCE
#include <arpa/inet.h>
#include <dlfcn.h>
#include <netinet/in.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>

typedef int (*bind_fn)(int, const struct sockaddr *, socklen_t);
typedef int (*connect_fn)(int, const struct sockaddr *, socklen_t);

static bind_fn real_bind;
static connect_fn real_connect;
static unsigned short target_port_net;
static unsigned short remap_port_net;
static int remap_active;
static int initialized;

static void shim_init(void) {
    if (initialized) {
        return;
    }
    real_bind = (bind_fn)dlsym(RTLD_NEXT, "bind");
    real_connect = (connect_fn)dlsym(RTLD_NEXT, "connect");
    target_port_net = htons(55142);

    const char *remap_env = getenv("REVOS_PORT_REMAP");
    if (remap_env && remap_env[0] != '\0') {
        int port = atoi(remap_env);
        if (port > 0 && port < 65536) {
            remap_port_net = htons((unsigned short)port);
            remap_active = 1;
        }
    }
    initialized = 1;
}

/* Rewrites addr in place if it's AF_INET, 127.0.0.1, port 55142. No-op otherwise. */
static void maybe_remap(struct sockaddr_in *sin) {
    if (!remap_active) {
        return;
    }
    if (sin->sin_family == AF_INET &&
        sin->sin_port == target_port_net &&
        sin->sin_addr.s_addr == htonl(INADDR_LOOPBACK)) {
        sin->sin_port = remap_port_net;
    }
}

int bind(int sockfd, const struct sockaddr *addr, socklen_t addrlen) {
    shim_init();
    if (addr && addr->sa_family == AF_INET && addrlen >= sizeof(struct sockaddr_in)) {
        struct sockaddr_in modified;
        memcpy(&modified, addr, sizeof(modified));
        maybe_remap(&modified);
        return real_bind(sockfd, (const struct sockaddr *)&modified, addrlen);
    }
    return real_bind(sockfd, addr, addrlen);
}

int connect(int sockfd, const struct sockaddr *addr, socklen_t addrlen) {
    shim_init();
    if (addr && addr->sa_family == AF_INET && addrlen >= sizeof(struct sockaddr_in)) {
        struct sockaddr_in modified;
        memcpy(&modified, addr, sizeof(modified));
        maybe_remap(&modified);
        return real_connect(sockfd, (const struct sockaddr *)&modified, addrlen);
    }
    return real_connect(sockfd, addr, addrlen);
}
