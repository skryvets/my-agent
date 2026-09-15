# Decisions - deploy to an Ubuntu server with systemd

## The steps of a deployment
**Q:** What does one run of the script do?
**A:** It stops the previous `my-agent` service, runs `go build`, installs the binary as a systemd service and starts it as a daemon. The script is `deploy/deploy.sh` and runs on the server, from the checkout.

## Build before stop
**Q:** Does the script stop the running bot before or after the build?
**A:** After. A build that fails exits the script before the stop, so the previous deployment stays up.

## Where the files go
**Q:** Where are the binary, the unit, the secrets and the runs?
**A:** The binary is `/usr/local/bin/my-agent`. The unit is `deploy/my-agent.service`, installed to `/etc/systemd/system/my-agent.service`. The variables are in `/etc/my-agent/env`, mode `0600`, owned by root, which systemd reads before it starts the process. The first run writes the file with empty values and stops. The runs are in `/var/lib/my-agent/state`, under the `StateDirectory=` of the unit.

## The user of the service
**Q:** Which user does the bot run as?
**A:** A system user `my-agent` with the home `/var/lib/my-agent`, in the `docker` group through `SupplementaryGroups=`. The `docker` group grants root-level access to the host, so the separate user only keeps the bot out of the files of other users.

## Restart
**Q:** When does systemd start the bot again?
**A:** Whenever it exits, after 5 seconds (`Restart=always`). A `systemctl stop` does not start it again. The stop waits 60 seconds for the tasks to end and their containers to be removed, then systemd kills the process.

## What the script does not do
**Q:** Does the script pull the code or run the tests?
**A:** No. `git pull` runs before the script, and the tests run before a push.
