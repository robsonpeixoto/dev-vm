#compdef dev-vm devvm
compdef _dev_vm dev-vm devvm 2>/dev/null

_dev_vm_names() {
	local -a names
	names=(${(f)"$($service __names 2>/dev/null)"})
	(($#names)) && compadd -a names
}

_dev_vm() {
	local curcontext="$curcontext" state line
	local -a commands
	commands=(
		'create:create a dev VM'
		'destroy:destroy a dev VM and its GitHub key'
		'list:list dev VMs with their SSH hostname'
		'completion:print the shell completion script'
		'version:print the version'
		'help:show usage'
	)

	_arguments -C '1: :->command' '*:: :->args'

	case $state in
	command)
		_describe -t commands 'dev-vm command' commands
		;;
	args)
		case $line[1] in
		create)
			_arguments \
				'-create-ssh-key=-[create and register a new key]:bool:(true false)' \
				'-dotfiles[bare repo to check out over the guest $HOME]:repo:' \
				'-no-dotfiles[skip dotfiles even when settings.json configures them]' \
				'-cpus[vCPUs for the VM]:count:' \
				'-memory[RAM in GiB]:gib:' \
				'-disk[disk size in GiB]:gib:' \
				'1:name:'
			;;
		destroy)
			_arguments \
				'-force[skip the confirmation prompt]' \
				'1:name:_dev_vm_names'
			;;
		completion)
			_arguments '1:shell:(bash zsh fish)'
			;;
		esac
		;;
	esac
}

if [ "$funcstack[1]" = "_dev_vm" ]; then
	_dev_vm "$@"
fi
