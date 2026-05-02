package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FindDescendantPID walks the process tree from parentPID using BFS and
// returns the first descendant whose comm string contains targetComm.
func FindDescendantPID(parentPID int, targetComm string) (int, error) {
	cmd := exec.Command("ps", "-o", "pid,ppid,comm")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	tree := make(map[int][]int)
	pidToComm := make(map[int]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			pid, _ := strconv.Atoi(fields[0])
			ppid, _ := strconv.Atoi(fields[1])
			comm := fields[2]
			tree[ppid] = append(tree[ppid], pid)
			pidToComm[pid] = comm
		}
	}

	queue := []int{parentPID}
	visited := make(map[int]bool)

	for len(queue) > 0 {
		currentPID := queue[0]
		queue = queue[1:]

		if visited[currentPID] {
			continue
		}
		visited[currentPID] = true

		if comm, ok := pidToComm[currentPID]; ok && strings.Contains(comm, targetComm) {
			return currentPID, nil
		}

		if children, ok := tree[currentPID]; ok {
			queue = append(queue, children...)
		}
	}

	return 0, fmt.Errorf("descendant process '%s' not found for parent PID %d", targetComm, parentPID)
}
