#!/usr/bin/env bash
set -euo pipefail

REPO="${BYTEMIND_REPO:-1024XEngineer/bytemind}"
VERSION="${BYTEMIND_VERSION:-}"
INSTALL_DIR="${BYTEMIND_INSTALL_DIR:-$HOME/.bytemind/bin}"

if [[ -n "${VERSION}" && "${VERSION}" != v* ]]; then
  VERSION="v${VERSION}"
fi

require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "missing required command: ${cmd}" >&2
    exit 1
  fi
}

download_file() {
  local url="$1"
  local destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${url}" -o "${destination}"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "${destination}" "${url}"
    return
  fi
  echo "missing required command: curl or wget" >&2
  exit 1
}

get_latest_version() {
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  local body
  body="$(mktemp)"
  trap 'rm -f "${body}"' RETURN
  download_file "${api_url}" "${body}"
  local tag
  tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${body}" | head -n1)"
  if [[ -z "${tag}" ]]; then
    echo "failed to resolve latest release tag from ${api_url}" >&2
    exit 1
  fi
  echo "${tag}"
}

normalize_os() {
  case "$(uname -s)" in
    Linux*) echo "linux" ;;
    Darwin*) echo "darwin" ;;
    *)
      echo "unsupported operating system: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

normalize_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

find_sha256_tool() {
  if command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    echo "shasum"
    return
  fi
  echo "missing required command: sha256sum or shasum" >&2
  exit 1
}

verify_checksum() {
  local archive_path="$1"
  local checksum_path="$2"
  local archive_name="$3"
  local sha_tool="$4"

  local expected actual
  expected="$(awk -v asset="${archive_name}" '$2 == asset {print $1; exit}' "${checksum_path}")"
  if [[ -z "${expected}" ]]; then
    echo "checksum entry not found for ${archive_name}" >&2
    exit 1
  fi

  if [[ "${sha_tool}" == "sha256sum" ]]; then
    actual="$(sha256sum "${archive_path}" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
  fi

  if [[ "${actual}" != "${expected}" ]]; then
    echo "checksum verification failed for ${archive_name}" >&2
    echo "expected: ${expected}" >&2
    echo "actual:   ${actual}" >&2
    exit 1
  fi
}

append_path_profile() {
  local install_dir="$1"
  if [[ ":${PATH}:" == *":${install_dir}:"* ]]; then
    return
  fi

  local shell_name profile
  shell_name="$(basename "${SHELL:-}")"
  case "${shell_name}" in
    zsh)
      profile="${HOME}/.zshrc"
      ;;
    bash)
      if [[ "$(uname -s)" == "Darwin" ]]; then
        profile="${HOME}/.bash_profile"
      else
        profile="${HOME}/.bashrc"
      fi
      ;;
    *)
      profile="${HOME}/.profile"
      ;;
  esac

  local line="export PATH=\"${install_dir}:\$PATH\""
  mkdir -p "$(dirname "${profile}")"
  touch "${profile}"
  if ! grep -Fq "${line}" "${profile}"; then
    printf "\n%s\n" "${line}" >> "${profile}"
    echo "added ${install_dir} to PATH in ${profile}"
  fi
}

main() {
  require_cmd tar
  local os arch sha_tool
  os="$(normalize_os)"
  arch="$(normalize_arch)"
  sha_tool="$(find_sha256_tool)"

  if [[ -z "${VERSION}" ]]; then
    VERSION="$(get_latest_version)"
  fi

  local asset="bytemind_${VERSION}_${os}_${arch}.tar.gz"
  local release_url="https://github.com/${REPO}/releases/download/${VERSION}"

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' EXIT

  local archive_path="${tmp_dir}/${asset}"
  local checksum_path="${tmp_dir}/checksums.txt"

  echo "downloading ${asset}"
  download_file "${release_url}/${asset}" "${archive_path}"
  download_file "${release_url}/checksums.txt" "${checksum_path}"
  verify_checksum "${archive_path}" "${checksum_path}" "${asset}" "${sha_tool}"

  tar -xzf "${archive_path}" -C "${tmp_dir}"
  local extracted_binary="${tmp_dir}/bytemind_${VERSION}_${os}_${arch}/bytemind"
  if [[ ! -f "${extracted_binary}" ]]; then
    echo "binary not found in archive: ${asset}" >&2
    exit 1
  fi
  chmod +x "${extracted_binary}"

  "${extracted_binary}" install -to "${INSTALL_DIR}" -add-to-path=false
  append_path_profile "${INSTALL_DIR}"

  echo
  echo "Bytemind is installed."
  echo "open a new terminal and run: bytemind chat"
}

main "$@"
