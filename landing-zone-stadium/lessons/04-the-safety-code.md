# Lesson 04: The safety code

*About 25 minutes. Site: Chapter 3, "The safety code", and "Break the rules" mode.*

## Why it matters

A stadium has one safety code. No fires in the stands. No blocked exits. Bags are checked. It is the same for
football, cricket and concerts. Nobody asks each event "would you like a fire exit?". The code is just there, and the
stewards enforce it.

In the cloud, the safety code is **Azure Policy**. You write the rules once, high up the tree, and every event below
gets them. If a team tries something unsafe, the request is stopped **before** it is built, with a clear error.

## How it works

- A **policy definition** is one rule. "Only UK regions." "No public IPs."
- An **initiative** (a policy set) bundles many rules into one safety code.
- An **assignment** pins the initiative to a place on the tree. Here: the `ct-landingzones` management group.
- **Rules flow down.** Every subscription under `ct-landingzones` (so `sub-events`, and every future event) gets the
  code automatically. The training ground (`ct-sandbox`) does not, so learners can practise there.
- The **effect** says what happens. `deny` stops the request. `audit` lets it through but reports it. `modify` fixes
  it on the way in (for example, adds a tag).

Our safety code, in `terraform/safety-code.tf`:

| Rule | Stadium words |
|---|---|
| UK regions only | the event happens in this stadium, not abroad |
| resource groups need `event`, `event-kit` and `cost-centre` tags | every booking has a name, a kit and someone who pays |
| no public IPs in events | no side doors to the street: use the gates |
| secure storage: no shared keys, TLS 1.2, no public blobs | the prop warehouse is locked |
| Key Vault with RBAC and purge protection | the club safe opens by wristband, and cannot be emptied by mistake |
| AKS with Entra RBAC and local accounts off | the team's dressing room uses stadium wristbands, not a spare key |
| PostgreSQL with no public access | the score book stays inside |

When a rule says no, Azure (and CloudTyke) answers with HTTP `403` and the code `RequestDisallowedByPolicy`, plus the
name of the assignment and the rule that said no.

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `terraform/safety-code.tf`. Find:
   - the `locals` map of rules (each one has `if` and `then`)
   - the initiative that bundles them
   - the assignment, scoped to `data.azurerm_management_group` for `ct-landingzones`
3. Notice the assignment name. A management group assignment name must be 24 characters or less.
4. Run the verifier. One of its checks is that the safety code is assigned at the management group:

   ```bash
   bash landing-zone-stadium/verify.sh
   ```

5. In the site, Chapter 3: sort each rule onto the right wall, then watch the rules flow down to every event.

## Break it

Try to break the code on purpose. In a toolbox shell (`cd azure-emulator && make shell`), get a token as in
Lesson 01, then:

1. **Build outside the UK.**

   ```bash
   S=/subscriptions/5b5c0000-0000-4000-8000-000000000004
   curl -sS -X PUT -H "Authorization: Bearer $T" -H 'content-type: application/json' \
     "$M$S/resourceGroups/rg-abroad?api-version=2024-11-01" \
     -d '{"location":"eastus","tags":{"event":"derby","event-kit":"football","cost-centre":"CC-7100"}}' | jq
   ```

   You get `RequestDisallowedByPolicy`. Read the message. It names the rule.

2. **Forget the tags.** Same call, with `"location":"uksouth"` and no `tags`. Denied again: no booking without a
   name, a kit and a payer.

3. **Open a side door.** Create a resource group with all three tags in `uksouth`, then try to create a public IP in
   it:

   ```bash
   curl -sS -X PUT -H "Authorization: Bearer $T" -H 'content-type: application/json' \
     "$M$S/resourceGroups/rg-sidedoor/providers/Microsoft.Network/publicIPAddresses/pip-sidedoor?api-version=2024-05-01" \
     -d '{"location":"uksouth","sku":{"name":"Standard"},"properties":{"publicIPAllocationMethod":"Static"}}' | jq
   ```

   Denied. The only way in is the gates.

4. **Try the same in the training ground.** Repeat step 1 in `sub-sandbox` (`5b5c0000-0000-4000-8000-000000000003`).
   It works, because the code is assigned at `ct-landingzones`, not at the root. That is on purpose.

## Check yourself

1. Where is the safety code assigned? Why there, and not on each subscription?
2. A new event subscription is added under `ct-landingzones` next month. Does it get the code? What do you need to do?
3. What is the difference between `deny` and `audit`?
4. Why does the training ground not get the same code?
5. A developer sees `RequestDisallowedByPolicy`. Where should they look first?

## In real Azure

The same: definitions, initiatives, assignments at management group scope, `deny`, `audit` and `modify` effects, and
the `RequestDisallowedByPolicy` error. Real Azure also has hundreds of **built-in** policies (for example "Allowed
locations"), regular **compliance scans** of resources that already exist, and **remediation tasks**. CloudTyke
enforces policy on every write, supports exemptions, and is enough to learn how the code flows down and how denials
look. The Azure landing zone reference design ships with a ready set of policies at each management group.

## Same idea in AWS and GCP

- **AWS**: **Service Control Policies (SCPs)** on OUs are the "deny" rules, and they flow down to every account.
  **Control Tower controls** bundle preventive and detective rules. A denial shows as `AccessDenied` with an
  "explicit deny in a service control policy" message.
- **GCP**: **Organization Policy constraints** (for example `gcp.resourceLocations`,
  `compute.vmExternalIpAccess`) set on the organisation or a folder are inherited by every project below.
