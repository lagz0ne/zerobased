package ports

import "hash/fnv"

// DeterministicPort hashes "<project>:<service>:<port>" into a stable port in [10000, 30000).
func DeterministicPort(project, service string, containerPort uint16) uint16 {
	h := fnv.New32a()
	h.Write([]byte(project))
	h.Write([]byte{':'})
	h.Write([]byte(service))
	h.Write([]byte{':'})
	h.Write([]byte{byte(containerPort >> 8), byte(containerPort)})
	return uint16(h.Sum32()%20000) + 10000
}
