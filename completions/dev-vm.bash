# mapfile needs bash 4, macOS ships 3.2; VM names have no spaces.
# shellcheck disable=SC2207
_dev_vm() {
    local cur prev cmd
    cur=${COMP_WORDS[COMP_CWORD]}
    prev=${COMP_WORDS[COMP_CWORD - 1]}
    cmd=${COMP_WORDS[1]}
    COMPREPLY=()

    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=($(compgen -W "create start stop destroy list completion version help" -- "$cur"))
        return
    fi

    case $cmd in
    create)
        case $prev in
        -dotfiles | --dotfiles | -cpus | --cpus | -memory | --memory | -disk | --disk)
            return
            ;;
        esac
        COMPREPLY=($(compgen -W "-create-ssh-key= -dotfiles -no-dotfiles -cpus -memory -disk -help" -- "$cur"))
        ;;
    start)
        if [ "${cur:0:1}" = "-" ]; then
            COMPREPLY=($(compgen -W "-help" -- "$cur"))
        else
            COMPREPLY=($(compgen -W "$("${COMP_WORDS[0]}" __names 2>/dev/null)" -- "$cur"))
        fi
        ;;
    stop | destroy)
        if [ "${cur:0:1}" = "-" ]; then
            COMPREPLY=($(compgen -W "-force -help" -- "$cur"))
        else
            COMPREPLY=($(compgen -W "$("${COMP_WORDS[0]}" __names 2>/dev/null)" -- "$cur"))
        fi
        ;;
    list)
        COMPREPLY=($(compgen -W "-help" -- "$cur"))
        ;;
    completion)
        COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
        ;;
    esac
}

complete -F _dev_vm dev-vm
complete -F _dev_vm devvm
