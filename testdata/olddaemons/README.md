Real `docker container inspect` and `docker image inspect` output from
Docker Engine 20.10.24 (API 1.41), 24.0.9 (1.43) and 26.1.4 (1.45), each
in a throwaway Lima VM (Ubuntu 22.04, docker-ce of that version from
download.docker.com, its own CLI). The containers come from `mk.sh`.
`TestOldDaemons` (runlike_test.go) checks each against
`<name>.golden.json` (with image data) and `<name>.stdin.golden.json`.

Shape changes against 29.9.3 seen in this data: `Config.MacAddress`
(gone in API 1.52; old data always has it), the top-level
`NetworkSettings` address fields (gone), `HostConfig.KernelMemory*`
(20.10 only), `Blkio*` null instead of [] (20.10), endpoint `DNSNames`
(new in 26.1) and `GwPriority` (new in 28), the container's short ID in
endpoint `Aliases` (before 26.1), `ImageManifestDescriptor` (new),
image `Container`/`ContainerConfig`/`VirtualSize` (gone in 26.1).
No field kept its name and changed type.
