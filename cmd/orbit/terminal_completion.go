package main

import (
	"fmt"
	"io"
	"strings"
)

func handleOrbitCompletion(args []string, stdout, stderr io.Writer) error {
	shell := "bash"
	if len(args) > 0 {
		shell = strings.ToLower(args[0])
	}

	switch shell {
	case "bash":
		fmt.Fprint(stdout, bashCompletionScript)
	case "zsh":
		fmt.Fprint(stdout, zshCompletionScript)
	case "fish":
		fmt.Fprint(stdout, fishCompletionScript)
	default:
		return fmt.Errorf("unsupported shell %q: choose bash, zsh, or fish", shell)
	}

	return nil
}

const bashCompletionScript = `_orbit_completion() {
    local cur prev words cword
    if declare -F _init_completion >/dev/null; then
        _init_completion || return
    else
        words=("${COMP_WORDS[@]}"); cword=${COMP_CWORD}
        cur=${COMP_WORDS[COMP_CWORD]}; prev=${COMP_WORDS[COMP_CWORD-1]}
    fi

    local commands="tui status context folders devices conflicts history deleted restore setup join network service stop storage doctor engine completion version help"
    local common_flags="--state --folder --json --help"

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
        return 0
    fi

    local cmd="${words[1]}"
    case "${cmd}" in
        folders)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "list add share pause resume relocate" -- "${cur}") )
                return 0
            fi
            ;;
        devices)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "list add invite requests approve decline endpoint" -- "${cur}") )
                return 0
            fi
            ;;
        service)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "status start stop restart enable disable" -- "${cur}") )
                return 0
            fi
            ;;
        conflicts)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "resolve" -- "${cur}") )
                return 0
            fi
            ;;
        completion)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
                return 0
            fi
            ;;
        network)
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=( $(compgen -W "status doctor automatic update set preview apply" -- "${cur}") )
                return 0
            fi
            ;;
    esac

    if [[ "${cur}" == -* ]]; then
        COMPREPLY=( $(compgen -W "${common_flags}" -- "${cur}") )
        return 0
    fi
}
complete -F _orbit_completion orbit
`

const zshCompletionScript = `#compdef orbit

_orbit() {
    local -a commands
    commands=(
        'tui:Keyboard interface'
        'status:Display concise folder and daemon synchronization status'
        'context:Inspect folder identity and path resolution'
        'folders:Manage synced folders and local roots'
        'devices:Manage enrolled devices and requests'
        'conflicts:List files requiring version or structure review'
        'history:List known versions and content availability'
        'deleted:Find deleted files available for restore'
        'restore:Restore a historical version of a file'
        'network:Review connection policy, diagnostics and cached routes'
        'setup:Create or adopt a synced folder'
        'join:Join an existing Orbit using a private invitation'
        'service:Manage background daemon service'
        'stop:Stop a daemon started outside the service'
        'storage:Inspect storage usage and maintenance'
        'doctor:Run actionable diagnostics'
        'engine:Low-level engine commands'
        'completion:Generate shell completion script'
        'version:Display version information'
        'help:Show command documentation'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    case "$words[2]" in
        folders)
            local -a subcmds
            subcmds=(
                'list:List registered synced folders'
                'add:Register a new folder'
                'share:Share folder with enrolled device'
                'pause:Pause synchronization'
                'resume:Resume synchronization'
                'relocate:Relocate folder to new path'
            )
            _describe 'subcommand' subcmds
            ;;
        devices)
            local -a subcmds
            subcmds=(
                'list:List known devices'
                'add:Create an invitation'
                'requests:Manage enrollment requests'
                'endpoint:Configure peer endpoint'
            )
            _describe 'subcommand' subcmds
            ;;
        service)
            local -a subcmds
            subcmds=(
                'status:Service status'
                'start:Start service'
                'stop:Stop service'
                'restart:Restart service'
                'enable:Enable service'
                'disable:Disable service'
            )
            _describe 'subcommand' subcmds
            ;;
        completion)
            local -a shells
            shells=('bash:Bash' 'zsh:Zsh' 'fish:Fish')
            _describe 'shell' shells
            ;;
        network)
            local -a subcmds
            subcmds=('status:Cached status' 'doctor:Bounded explicit probes' 'automatic:Use the packaged service profile' 'update:Review a newer packaged profile' 'set:Change policy with one confirmation' 'preview:Review policy to a file' 'apply:Apply reviewed policy file')
            _describe 'subcommand' subcmds
            ;;
        *)
            _arguments \
                '--state[Agent state directory]:path:_files -/' \
                '--folder[Target synced folder]:name:' \
                '--json[Output structured JSON]' \
                '--help[Show help]'
            ;;
    esac
}

_orbit "$@"
`

const fishCompletionScript = `function __fish_orbit_no_subcommand
    for i in (commandline -opc)
        if contains -- $i tui status context folders devices conflicts history deleted restore setup join network service stop storage doctor engine completion version help
            return 1
        end
    end
    return 0
end

complete -c orbit -n '__fish_orbit_no_subcommand' -a 'tui' -d 'Keyboard interface'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'status' -d 'Display concise status'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'context' -d 'Inspect folder identity and path resolution'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'folders' -d 'Manage synced folders'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'devices' -d 'Manage enrolled devices'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'conflicts' -d 'List files requiring version review'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'history' -d 'List known versions'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'deleted' -d 'Find deleted files available for restore'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'restore' -d 'Restore a historical version'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'engine' -d 'Low-level engine commands'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'setup' -d 'Create or adopt a synced folder'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'join' -d 'Join an existing Orbit'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'network' -d 'Status, diagnostics and policy controls'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'service' -d 'Manage background daemon service'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'stop' -d 'Stop a daemon started outside the service'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'storage' -d 'Inspect storage usage and maintenance'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'doctor' -d 'Run actionable diagnostics'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'completion' -d 'Generate shell completion script'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'version' -d 'Display version metadata'
complete -c orbit -n '__fish_orbit_no_subcommand' -a 'help' -d 'Show command documentation'

complete -c orbit -n '__fish_seen_subcommand_from folders' -a 'list add share pause resume relocate'
complete -c orbit -n '__fish_seen_subcommand_from devices' -a 'list add invite requests approve decline endpoint'
complete -c orbit -n '__fish_seen_subcommand_from service' -a 'status start stop restart enable disable'
complete -c orbit -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
complete -c orbit -n '__fish_seen_subcommand_from network' -a 'status doctor automatic update set preview apply'

complete -c orbit -l state -d 'Agent state directory'
complete -c orbit -l folder -d 'Target synced folder'
complete -c orbit -l json -d 'Output structured JSON'
complete -c orbit -l help -d 'Show command help'
`
