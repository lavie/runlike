#!/bin/bash
# homebrew-formula.sh VERSION SHA256SUMS: print Formula/runlike.rb for the
# tap lavie/homebrew-tap. It installs the release binaries of VERSION
# (without v), checked against the sums in the release's SHA256SUMS.
set -euo pipefail
v=${1:?usage: homebrew-formula.sh VERSION SHA256SUMS}
sums=${2:?usage: homebrew-formula.sh VERSION SHA256SUMS}
sum() {
	local s
	s=$(awk -v f="$1" '$2 == f {print $1}' "$sums")
	[[ $s =~ ^[0-9a-f]{64}$ ]] || { echo "no sha256 for $1 in $sums" >&2; exit 1; }
	echo "$s"
}
mac=$(sum runlike-darwin-arm64)
amd=$(sum runlike-linux-amd64)
arm=$(sum runlike-linux-arm64)
url="https://github.com/lavie/runlike/releases/download/v$v"
cat <<EOF
# Written by runlike's release workflow (scripts/homebrew-formula.sh in
# lavie/runlike); edits here are overwritten by the next release.
class Runlike < Formula
  desc "Print the docker run command that recreates an existing container"
  homepage "https://github.com/lavie/runlike"
  license "BSD-3-Clause"

  on_macos do
    depends_on arch: :arm64 # no Intel macOS binary
    on_arm do
      url "$url/runlike-darwin-arm64"
      sha256 "$mac"
    end
  end
  on_linux do
    on_intel do
      url "$url/runlike-linux-amd64"
      sha256 "$amd"
    end
    on_arm do
      url "$url/runlike-linux-arm64"
      sha256 "$arm"
    end
  end

  def install
    bin.install Dir["runlike-*"].first => "runlike"
  end

  test do
    assert_match "runlike v#{version}", shell_output("#{bin}/runlike --version")
    inspect = '[{"Name":"/web","Config":{"Image":"nginx:1.27","Env":["A=b c"]},"HostConfig":{}}]'
    assert_equal "docker run --name=web '--env=A=b c' --detach nginx:1.27",
                 pipe_output("#{bin}/runlike --stdin 2>/dev/null", inspect, 0).chomp
  end
end
EOF
