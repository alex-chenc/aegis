// Safe CVE-2026-31431 runtime detection trigger.
// This program only exercises the AF_ALG socket/bind + splice syscall pattern
// used by the detector. It does not modify privileged files or attempt
// privilege escalation.
#define _GNU_SOURCE

#include <errno.h>
#include <fcntl.h>
#include <linux/if_alg.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

int main(void) {
	int alg_fd = socket(AF_ALG, SOCK_SEQPACKET, 0);
	if (alg_fd < 0) {
		fprintf(stderr, "socket(AF_ALG) failed: %s\n", strerror(errno));
		return 1;
	}

	struct sockaddr_alg sa;
	memset(&sa, 0, sizeof(sa));
	sa.salg_family = AF_ALG;
	strncpy((char *)sa.salg_type, "aead", sizeof(sa.salg_type) - 1);
	strncpy((char *)sa.salg_name, "gcm(aes)", sizeof(sa.salg_name) - 1);

	if (bind(alg_fd, (struct sockaddr *)&sa, sizeof(sa)) < 0) {
		fprintf(stderr, "bind(AF_ALG/aead/gcm(aes)) failed: %s\n", strerror(errno));
	}

	int pipefd[2];
	if (pipe(pipefd) != 0) {
		fprintf(stderr, "pipe failed: %s\n", strerror(errno));
		close(alg_fd);
		return 1;
	}

	const char marker[] = "aegis-cve-2026-31431-safe-trigger\n";
	if (write(pipefd[1], marker, sizeof(marker) - 1) < 0) {
		fprintf(stderr, "write pipe failed: %s\n", strerror(errno));
	}
	close(pipefd[1]);

	int null_fd = open("/dev/null", O_WRONLY);
	if (null_fd < 0) {
		fprintf(stderr, "open /dev/null failed: %s\n", strerror(errno));
		close(pipefd[0]);
		close(alg_fd);
		return 1;
	}

	ssize_t spliced = splice(pipefd[0], NULL, null_fd, NULL, sizeof(marker) - 1, 0);
	if (spliced < 0) {
		fprintf(stderr, "splice failed: %s\n", strerror(errno));
	}

	close(null_fd);
	close(pipefd[0]);
	close(alg_fd);

	printf("safe trigger completed, spliced=%zd\n", spliced);
	return 0;
}
