function __dev_vm_names
    command dev-vm __names 2>/dev/null
end

set -l dev_vm_commands create destroy list completion version help

complete -c dev-vm -f

complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a create -d 'create a dev VM'
complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a destroy -d 'destroy a dev VM and its GitHub key'
complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a list -d 'list dev VMs with their SSH hostname'
complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a completion -d 'print the shell completion script'
complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a version -d 'print the version'
complete -c dev-vm -n "not __fish_seen_subcommand_from $dev_vm_commands" -a help -d 'show usage'

complete -c dev-vm -n '__fish_seen_subcommand_from create' -o create-ssh-key -r -a 'true false' -d 'create and register a new key'
complete -c dev-vm -n '__fish_seen_subcommand_from create' -o dotfiles -r -d 'bare repo to check out over the guest $HOME'
complete -c dev-vm -n '__fish_seen_subcommand_from create' -o no-dotfiles -d 'skip dotfiles even when settings.json configures them'
complete -c dev-vm -n '__fish_seen_subcommand_from create' -o cpus -r -d 'vCPUs for the VM'
complete -c dev-vm -n '__fish_seen_subcommand_from create' -o memory -r -d 'RAM in GiB'
complete -c dev-vm -n '__fish_seen_subcommand_from create' -o disk -r -d 'disk size in GiB'

complete -c dev-vm -n '__fish_seen_subcommand_from destroy' -a '(__dev_vm_names)' -d 'dev VM'
complete -c dev-vm -n '__fish_seen_subcommand_from destroy' -o force -d 'skip the confirmation prompt'

complete -c dev-vm -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish' -d shell
