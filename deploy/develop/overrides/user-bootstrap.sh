#!/usr/bin/env bash
set -euo pipefail

USER_NAME="spigell"
USER_UID=7000
USER_HOME="/home/${USER_NAME}"
CODEX_CONFIG_SOURCE="/project/deploy/develop/overrides/codex-config.toml"
CODEX_CONFIG_DEST="${USER_HOME}/.codex/config.toml"
HH_TOKEN_SECRET_SOURCE="/secrets/hh-token/hh-token.txt"
HH_TOKEN_SECRET_DEST="${USER_HOME}/.hh-token.txt"

ensure_user() {
  if ! id -u "${USER_NAME}" >/dev/null 2>&1; then
    echo ">>> Creating user ${USER_NAME} (${USER_UID})"
    useradd -m -u "${USER_UID}" -s /bin/bash "${USER_NAME}"
  fi
  
  cat >> "/etc/profile" <<'EOF'
export PATH=/usr/local/share/fnm/aliases/default/bin:/usr/local/share/pyenv/shims:/usr/local/share/pyenv/bin:/usr/local/share/dotnet:~/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
EOF

  mkdir -p "${USER_HOME}"/{.home,.cache/gomod,.cache/gobuild,.cache/gopath,go/bin,.codex}

  rm -rf "${CODEX_CONFIG_DEST}"
  ln -s "${CODEX_CONFIG_SOURCE}" "${CODEX_CONFIG_DEST}"
  ln -s "${HH_TOKEN_SECRET_SOURCE}" "${HH_TOKEN_SECRET_DEST}"
  
  chown -R "${USER_UID}:${USER_UID}" "${USER_HOME}"

  cat >> "${USER_HOME}/.bashrc" <<'EOF'
# Completions (ignore errors if not present)
[ -f /usr/share/bash-completion/completions/make ] && source /usr/share/bash-completion/completions/make

EOF
  chown "${USER_UID}:${USER_UID}" "${USER_HOME}/.bashrc"
}

run_as_user() { # $@ = command
  su "${USER_NAME}" -l -s /bin/bash -c "$*"
}


case "${1:-}" in
  main)
    ensure_user
    echo ">>> Codex (local) for ${USER_NAME}"
    run_as_user "sleep infinity"
    # run_as_user "cd /project && whoami && codex"
    ;;
  *)
    echo "Usage: $0 {main}" >&2
    exit 1
    ;;
esac
