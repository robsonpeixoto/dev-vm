# Shared shell helpers for the dev-vm provision scripts. Sourced, never run:
# `. /usr/local/lib/dev-vm/lib.sh`. Shipped as a `mode: data` file, so it is in
# place before the first system script runs.

# packages_missing succeeds when at least one of the named packages is not
# installed. Provision scripts run on every boot, so this is what keeps the
# second boot off the network: nothing missing means no keyring download, no
# apt-get update and no install.
packages_missing() {
	for package in "$@"; do
		dpkg-query -W -f='${db:Status-Status}' "$package" 2>/dev/null |
			grep -qx installed || return 0
	done
	return 1
}
