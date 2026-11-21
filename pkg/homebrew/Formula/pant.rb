class Pant < Formula
  desc "Modern Baidu Netdisk CLI & interactive TUI client"
  homepage "https://github.com/qwerprog/pan-tools"
  version "5.0.1"
  license "Apache-2.0"

  on_intel do
    url "https://github.com/qwerprog/pan-tools/releases/download/v5.0.1/pant-v5.0.1-darwin-amd64.zip"
    sha256 "43e429806e6635a1c692db674e986d9b53cce8a28426778f7c56fd0180135f3e"
  end

  on_arm do
    url "https://github.com/qwerprog/pan-tools/releases/download/v5.0.1/pant-v5.0.1-darwin-arm64.zip"
    sha256 "77b776f13d27a782128da90ea5da2a5c7a5dd9476a4d1a8b39250612add0e078"
  end

  def install
    bin.install "pant"
    doc.install "README.md"
    prefix.install "LICENSE", "NOTICE"
  end

  test do
    assert_match "5.0.1", shell_output("#{bin}/pant --version")
  end
end
