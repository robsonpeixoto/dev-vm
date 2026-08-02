require "download_strategy"
require "utils/github"

# Downloads a release asset from a private GitHub repository.
# Credentials come from HOMEBREW_GITHUB_API_TOKEN, the `gh` CLI login, or the
# macOS keychain (in that order).
class GitHubPrivateReleaseDownloadStrategy < CurlDownloadStrategy
  def initialize(url, name, version, **meta)
    super
    match = url.match(%r{^https://github\.com/([^/]+)/([^/]+)/releases/download/([^/]+)/(.+)$})
    raise CurlDownloadStrategyError.new(url, "not a GitHub release URL") if match.nil?

    @owner, @repo, @tag, @filename = match.captures
  end

  private

  def _fetch(url:, resolved_url:, timeout:)
    token = GitHub::API.credentials
    if token.blank?
      message = "no GitHub credentials: set HOMEBREW_GITHUB_API_TOKEN or run `gh auth login`"
      raise CurlDownloadStrategyError.new(url, message)
    end

    curl_download asset_url,
                  "--header", "Accept: application/octet-stream",
                  "--header", "Authorization: Bearer #{token}",
                  to: temporary_path, timeout: timeout
  end

  def asset_url
    release = GitHub::API.open_rest("https://api.github.com/repos/#{@owner}/#{@repo}/releases/tags/#{@tag}")
    asset = release["assets"].find { |a| a["name"] == @filename }
    raise CurlDownloadStrategyError.new(url, "#{@filename} not in release #{@tag}") if asset.nil?

    asset["url"]
  end
end

class DevVm < Formula
  desc "Isolated Lima dev VM for macOS with rootless Docker and GitHub SSH"
  homepage "https://github.com/robsonpeixoto/dev-vm"
  version "0.1.0"

  depends_on "gh"
  depends_on "lima"

  on_macos do
    on_arm do
      url "https://github.com/robsonpeixoto/dev-vm/releases/download/v#{version}/dev-vm_#{version}_darwin_arm64.tar.gz",
          using: GitHubPrivateReleaseDownloadStrategy
      sha256 "c603a5f456b05926e21564a3140a814b2dc02bcb9e25db1098b52ce2232ffbe3"
    end
    on_intel do
      url "https://github.com/robsonpeixoto/dev-vm/releases/download/v#{version}/dev-vm_#{version}_darwin_amd64.tar.gz",
          using: GitHubPrivateReleaseDownloadStrategy
      sha256 "c53e2692f72a469c86ce3ee3763172d379afaae677131a3c905513b3713cb93a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/robsonpeixoto/dev-vm/releases/download/v#{version}/dev-vm_#{version}_linux_arm64.tar.gz",
          using: GitHubPrivateReleaseDownloadStrategy
      sha256 "2cb5981e72b979c34e1ecd58e3b04511381e9d2f1f8cb783cfafd9124bc11f3d"
    end
    on_intel do
      url "https://github.com/robsonpeixoto/dev-vm/releases/download/v#{version}/dev-vm_#{version}_linux_amd64.tar.gz",
          using: GitHubPrivateReleaseDownloadStrategy
      sha256 "215c6a171b9c164f283edf0369b69006603e5fa86d5c71fe17901026a0ab0acd"
    end
  end

  def install
    bin.install "dev-vm"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dev-vm version")
  end
end
