# Lesson 06: Swap the event

*About 25 minutes. Site: Chapter 6, "The pitch and the changeover" (the event switch).*

## A story first: the overnight changeover

Picture an indoor arena. On Tuesday night it hosts an ice hockey game. On Wednesday night it hosts basketball.

The ice does not melt. When the hockey crowd leaves, a crew covers the ice with insulated panels. On top of the panels
they build a basketball court. Hoops roll in, seats move, the scoreboard changes. By morning it is a basketball arena.
The next night it can go back again.

The crew works from a checklist, every time, in the same order. The base (the ice) stays. Only the layer on top
changes.

Bigger stadiums do the same with bigger machines. Some stadiums, such as the GelreDome (the first, in 1998), the
Veltins-Arena, the Sapporo Dome and Tottenham Hotspur Stadium, can slide the whole grass pitch out of the building.
Tottenham's pitch splits into three parts. Underneath is a hard floor for NFL games and concerts. The change takes a
few hours. The grass is protected, and the layer on top is swapped.

In the cloud:

- the **base that stays** is the landing zone: gates, seats, control room, safety code
- the **overnight crew** is the **pipeline**
- their **checklist** is the **runbook** (and the Terraform code)
- the **grass that is protected** is your precious workload and data

## Why it matters

If a change of event means rebuilding the network, the firewall or the rules, every change is risky and slow. If a
change of event touches **only the pitch**, it is fast and safe. You can prove it before you do it: the plan shows
exactly what will change.

## How it works

Each event in `terraform/variables.tf` names its kit. The seats in `seats.tf` do not look at the kit at all. Only
`pitch.tf` reads it. So if you change one event's kit:

- the **seats** stay the same (they depend on the event, not the kit)
- the **gates**, the **control room** and the **safety code** stay the same (they do not depend on events)
- only that **event's pitch** is replaced

`tofu plan` tells you what would change before anything happens. `tofu plan -json` gives the same answer as data, so
a script can check it. That is what `verify.sh` does.

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `terraform/variables.tf`. Change `derby` from `kit = "football"` to `kit = "concert"`.
3. Make a plan in the toolbox, the same way `scripts/up.sh` runs `tofu` (look at the top of the script for the
   command), but run `plan` instead of `apply`. Or pass the change on the command line with `-var`, without editing
   the file.
4. Read the plan. Make two lists:
   - **What changed**: for example the derby node pool sizes, the derby media storage rule.
   - **What stayed the same**: the hub, the firewall, the derby seats, the summer event, the control room, the safety
     code.
5. Run the verifier:

   ```bash
   bash landing-zone-stadium/verify.sh
   ```

   One check makes this exact change and fails if the plan touches anything but derby's pitch.

6. Apply the change, then change it back to `football` and apply again. Two changeovers, no gates rebuilt.
7. In the site, flip the event switch: Football, Cricket, Concert. Read the "What changed / What stayed the same" list.
   Parts that stayed get an "unchanged" badge.

## Break it

1. **A pitch that leans on the seats.** Imagine the concert kit needs a different spoke address range, and someone
   moves that setting into the seats. Now a kit change also changes the seats. Run a plan and see the spoke being
   replaced. That breaks every app in the event. Lesson: keep "what stays" and "what changes" in separate places.
2. **A change made by hand.** In the dashboard, or with a `PUT`, change the derby node pool size by hand. Run a plan.
   Terraform wants to put it back. This is **drift**. The crew's checklist is the truth, not a quick fix at midnight.

## Check yourself

1. In the arena story, what is the ice? What are the panels? What is the crew's checklist?
2. Which Terraform file is the only one that reads the kit?
3. You change `summer` from cricket to concert. Should the derby event change at all?
4. Why check the plan with a script (`verify.sh`) and not only by eye?
5. Cricket keeps years of stats in a database. If you swap summer to a concert, what happens to that data? How would
   you protect it?

## In real Azure

The same. Real teams run this through a **pipeline**: a pull request shows the plan, a reviewer checks it, and a merge
applies it. With **GitHub OIDC** the pipeline signs in to Azure without any stored secret. Policies such as
`prevent_destroy` or Azure **resource locks** protect data that must outlast an event (Question 5). CloudTyke gives the
same plan output, so the same checks work.

## Same idea in AWS and GCP

- **AWS**: the crew is **CodePipeline** or **GitHub Actions with OIDC**. The plan check is the same with Terraform. AWS
  CloudFormation has **change sets** for the same "show me first" idea.
- **GCP**: the crew is **Cloud Build** or GitHub Actions with **Workload Identity Federation**. The plan check is the
  same with Terraform.
