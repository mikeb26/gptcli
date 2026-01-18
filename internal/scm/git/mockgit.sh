#!/bin/sh
set -u

log() {
    if [ "${MOCK_GIT_LOG:-}" != "" ]; then
        printf '%s\n' "$*" >> "${MOCK_GIT_LOG}"
    fi
}

sanitize() {
    # Replace characters that are not allowed in environment variable names.
    echo "$1" | tr '.-' '__'
}

envget() {
    # Usage: envget VAR
    # Prints the variable value if set; prints empty otherwise.
    eval "printf '%s' \"\${$1-}\""
}

if [ "${MOCK_GIT_SLEEP_SECS:-}" != "" ]; then
    sleep "${MOCK_GIT_SLEEP_SECS}"
fi

# Strip optional "-C <dir>" prefix.
if [ "${1:-}" = "-C" ]; then
    shift 2
fi

cmd="${1:-}"
shift || true
log "$cmd $*"

case "$cmd" in
    rev-parse)
        case "${1:-}" in
            --is-inside-work-tree)
                printf '%s\n' "${MOCK_GIT_INSIDE:-true}"
                exit 0
                ;;
            --show-toplevel)
                printf '%s\n' "${MOCK_GIT_TOPLEVEL:-}"
                exit 0
                ;;
            --absolute-git-dir)
                printf '%s\n' "${MOCK_GIT_ABS_GIT_DIR:-}"
                exit 0
                ;;
            --short)
                # rev-parse --short HEAD
                printf '%s\n' "${MOCK_GIT_HEAD_SHORT:-abc123}"
                exit 0
                ;;
            --verify)
                # rev-parse --verify --quiet refs/stash
                if [ "${MOCK_GIT_HAS_STASH:-0}" = "1" ]; then
                    exit 0
                fi
                exit 1
                ;;
        esac
        ;;

    check-ignore)
        # check-ignore -q . => exit 0 if ignored, 1 if not ignored
        if [ "${MOCK_GIT_CHECK_IGNORE:-0}" = "1" ]; then
            exit 0
        fi
        exit 1
        ;;

    symbolic-ref)
        if [ "${MOCK_GIT_SYMBOLIC_REF_EXIT:-0}" != "0" ]; then
            printf '%s\n' "symbolic-ref failed" 1>&2
            exit "${MOCK_GIT_SYMBOLIC_REF_EXIT}"
        fi
        printf '%s\n' "${MOCK_GIT_BRANCH:-main}"
        exit 0
        ;;

    describe)
        if [ "${MOCK_GIT_DESCRIBE_EXIT:-0}" != "0" ]; then
            printf '%s\n' "describe failed" 1>&2
            exit "${MOCK_GIT_DESCRIBE_EXIT}"
        fi
        printf '%s\n' "${MOCK_GIT_DESCRIBE:-v0.0.0}"
        exit 0
        ;;

    status)
        if [ "${MOCK_GIT_STATUS_EXIT:-0}" != "0" ]; then
            printf '%s\n' "status failed" 1>&2
            exit "${MOCK_GIT_STATUS_EXIT}"
        fi
        printf '%s' "${MOCK_GIT_STATUS_OUT:-}"
        exit 0
        ;;

    config)
        case "${1:-}" in
            --bool)
                key="${2:-}"
                s="$(sanitize "$key")"
                var="MOCK_GIT_CONFIG_BOOL_${s}"
                v="$(envget "$var")"
                if [ "$v" = "" ]; then
                    printf '%s\n' "not found" 1>&2
                    exit 1
                fi
                printf '%s\n' "$v"
                exit 0
                ;;
            --get)
                key="${2:-}"
                s="$(sanitize "$key")"
                var="MOCK_GIT_CONFIG_GET_${s}"
                v="$(envget "$var")"
                if [ "$v" = "" ]; then
                    printf '%s\n' "not found" 1>&2
                    exit 1
                fi
                printf '%s\n' "$v"
                exit 0
                ;;
        esac
        ;;

    ls-files)
        case "${1:-}" in
            --others)
                printf '%s' "${MOCK_GIT_UNTRACKED:-}"
                exit 0
                ;;
            --unmerged)
                printf '%s' "${MOCK_GIT_UNMERGED:-}"
                exit 0
                ;;
        esac
        ;;

    add)
        if [ "${MOCK_GIT_ADD_EXIT:-0}" != "0" ]; then
            printf '%s\n' "add failed" 1>&2
            exit "${MOCK_GIT_ADD_EXIT}"
        fi
        exit 0
        ;;

    diff)
        # diff --cached --name-only
        printf '%s' "${MOCK_GIT_STAGED_FILES:-}"
        exit 0
        ;;

    difftool)
        if [ "${MOCK_GIT_DIFFTOOL_EXIT:-0}" != "0" ]; then
            printf '%s\n' "difftool failed" 1>&2
            exit "${MOCK_GIT_DIFFTOOL_EXIT}"
        fi
        exit 0
        ;;

    commit)
        if [ "${MOCK_GIT_COMMIT_EXIT:-0}" != "0" ]; then
            printf '%s\n' "commit failed" 1>&2
            exit "${MOCK_GIT_COMMIT_EXIT}"
        fi
        exit 0
        ;;
esac

printf '%s\n' "unsupported mock git command: $cmd" 1>&2
exit 2
