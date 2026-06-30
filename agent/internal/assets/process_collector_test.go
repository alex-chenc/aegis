package assets

import "testing"

const testProcessContainerID = "f3e1ce081b167859bacb51b22443f0f9ceaf9aafee0feb9b9e369eb670e506ce"

func TestParseContainerIdentityFromProcessCgroupDockerScope(t *testing.T) {
	runtime, id := parseContainerIdentityFromProcessCgroup("0::/system.slice/docker-" + testProcessContainerID + ".scope\n")
	if runtime != "docker" || id != testProcessContainerID {
		t.Fatalf("runtime/id = %q/%q, want docker/%s", runtime, id, testProcessContainerID)
	}
}

func TestParseContainerIdentityFromProcessCgroupHostProcess(t *testing.T) {
	runtime, id := parseContainerIdentityFromProcessCgroup("0::/\n")
	if runtime != "" || id != "" {
		t.Fatalf("runtime/id = %q/%q, want empty host identity", runtime, id)
	}
}
