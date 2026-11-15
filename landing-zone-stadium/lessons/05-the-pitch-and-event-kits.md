# Lesson 05: The pitch and event kits

*About 20 minutes. Site: Chapter 6, "The pitch and the changeover".*

## Why it matters

The pitch is the only part of the stadium that changes. Football needs grass and goals. Cricket needs an oval and a
strip in the middle. A concert needs a stage and a standing area. If the crew had to work out the setup from scratch
each time, changeovers would be slow and full of mistakes.

So the crew keeps a **kit** for each kind of event: a tested list of what goes on the pitch. In the cloud, a kit is a
ready set of app parts that fits on the landing zone's seats.

## How it works

The pitch is the `ct-landingzones` management group. Each event is one landing zone. In CloudTyke there is one
subscription for events, `sub-events`, and each event gets its own resource group and spoke inside it.

There are three kits. Each one fits a different shape of event:

| Kit | Shape of the event | What the pitch needs |
|---|---|---|
| Football ("Saturday Derby") | 90 minutes, huge rush at kick-off and half-time | AKS with a big autoscaling pool (for example min 3, max 30) for tickets and the fan app; highlight clips kept a short time |
| Cricket ("Summer Test") | can last up to 5 days, rain stops play | small steady AKS pool; PostgreSQL for scorecards and stats kept for years; Blob storage for ball-tracking replays that moves to cool, then archive |
| Concert ("Neon Nights") | about 3 hours, big rush at doors-open | AKS autoscaling pool; storage for stage media deleted after 30 days; no database |

In Terraform:

- `terraform/pitch.tf` has a `locals.kits` **catalogue**: one entry per kit, with its pool sizes, its storage rules
  and whether it has a database.
- `terraform/variables.tf` lists the events. Each event names its kit:

  ```hcl
  events = {
    derby  = { kit = "football", ... }
    summer = { kit = "cricket", ... }
  }
  ```

- `pitch.tf` looks up each event's kit and builds only what that kit asks for.

Event keys are short: lowercase letters and numbers, at most 10 characters. Storage account and Key Vault names have
length limits, and the key goes into those names. A `validation` block checks this for you.

Two facts from the emulator worth knowing:

- AKS autoscaling needs `min_count` and `max_count`, and the node count must sit between them.
- PostgreSQL private access needs a subnet delegated to `Microsoft.DBforPostgreSQL/flexibleServers` and a private DNS
  zone ending in `.postgres.database.azure.com`.

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `terraform/pitch.tf`. Read the `kits` catalogue. For each kit, write down: node pool min and max, storage
   lifecycle rule, database yes or no.
3. Open `terraform/variables.tf`. Which kit does `derby` use? Which does `summer` use?
4. Look at the outputs. They show each event's kit, resource group, VNet and AKS cluster name. (`up.sh` prints them
   at the end.)
5. In the dashboard (<http://localhost:4310>), open `sub-events` and find the derby and summer resource groups. Compare
   them. Same seats, different pitch.
6. In the site, Chapter 6: build tonight's kit from the pieces. Each piece shows "Changes per event".

## Break it

1. **Bad event key.** Add an event called `Big-Final-2026` in `variables.tf` and run a plan. The `validation` block
   stops it before anything is built. Change it to `final26`.
2. **Count outside the bounds.** In the football kit, set the starting node count to 40 while `max_count` is 30. The
   plan or the apply fails, because the count must sit between min and max.
3. **Kit that does not exist.** Set `kit = "rugby"`. The lookup in `locals.kits` fails. A good catalogue only offers
   kits the crew has tested.

Put everything back after each step.

## Check yourself

1. Which part of the stadium is the only part that changes?
2. Why does cricket have a database but the concert does not?
3. Why does the football kit have a much bigger `max_count` than the cricket kit?
4. Why must event keys be short?
5. Where does the list of kits live, and where does each event pick one?

## In real Azure

The same shape works in real Azure. Teams often get a kit as a **Terraform module** or a **template spec**, and a
platform team keeps the catalogue. Many companies give each event (each app) its own **subscription** through
**subscription vending**, not just its own resource group. We share `sub-events` here to keep the lab small. Real AKS,
PostgreSQL and storage lifecycle rules behave as described; in CloudTyke they are modelled and validated, and AKS can
be backed by a real k3s cluster with `make up-k3s`.

## Same idea in AWS and GCP

- **AWS**: each event is an **account** under the Workloads OU. **Account Factory for Terraform (AFT)** books a new
  one. The kit could use **EKS**, **RDS** and **S3** lifecycle rules (to Glacier for archive).
- **GCP**: each event is a **project** under the `landing-zones` folder. **Project Factory** books a new one. The kit
  could use **GKE**, **Cloud SQL** and **Cloud Storage** lifecycle rules (to Coldline or Archive).
