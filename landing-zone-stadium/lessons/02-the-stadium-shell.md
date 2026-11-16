# Lesson 02: The stadium shell

*About 20 minutes. Site: Chapter 1, "Build the stadium".*

## Why it matters

Every event needs gates, a control room, a kit room and stewards. If every event built its own, you would have ten
sets of gates, ten control rooms and ten ways of doing security. Some would be good. Some would be bad. Nobody could
watch them all.

So you build the shell **once**, and every event shares it. In the cloud this shared part is the **platform**. A small
platform team owns it. App teams use it, but do not rebuild it.

## How it works

The shell lives under the `ct-platform` management group. It has two subscriptions:

| Stadium part | What it does | CloudTyke |
|---|---|---|
| Entry gates, turnstiles, bag checks | one way in, everything checked | `sub-connectivity`: hub VNet, Azure Firewall |
| Staff door | a safe way in for staff only | Azure Bastion, in `sub-connectivity` |
| Signs around the ground | help everyone find the right place | private DNS zones, in `sub-connectivity` |
| Kit room | shared gear for every event | a shared Container Registry, in `sub-connectivity` |
| Control room and CCTV | watch everything, keep the tapes | `sub-management`: Log Analytics |

Why two subscriptions and not one? Different jobs, different people. The network team looks after the gates. The
operations team looks after the control room. Keeping them apart means a mistake in one does not break the other, and
you can give each team access only to its own part.

The shell is **never rebuilt** for an event. That is the whole point.

## Do it

1. If CloudTyke is not running yet, start it from the repo root: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `landing-zone-stadium/terraform/platform.tf`. Find:
   - the hub VNet and its firewall subnet (the gates)
   - the Bastion subnet (the staff door)
   - the private DNS zones (the signs)
   - the Container Registry (the kit room)
   - the Log Analytics workspace (the control room)
3. Notice the `provider = azurerm.connectivity` and `provider = azurerm.management` lines. Each part is built in its
   own subscription. That is one provider per subscription.
4. Open the dashboard (<http://localhost:4310>), go to Resources, and pick `sub-connectivity`. Find the hub network.
   Then pick `sub-management` and find the workspace.
5. In the site, Chapter 1, place the gates, the control room and the kit room. Each one shows **"Never rebuilt"**.

## Break it

Imagine the derby team wants "their own gates". Ask: what goes wrong?

- Fans now have two ways in. Only one has the bag check you trust.
- The control room cannot see the new gates.
- The next event will want its own gates too.

Now look at it the other way. Open `terraform/seats.tf` and check: does any event build a firewall or a public IP?
It should not. Every event's traffic goes to the one hub firewall. (In Lesson 04 the safety code makes sure of it.)

## Check yourself

1. Which management group holds the stadium shell?
2. What lives in `sub-connectivity`? What lives in `sub-management`?
3. Why is the staff door (Bastion) better than giving every server its own door to the street?
4. Why split the shell into two subscriptions?
5. Name one thing the shell should **never** do for a single event.

## In real Azure

The Azure landing zone design calls these the **connectivity** and **management** subscriptions, under a
**Platform** management group. Real designs often add an **identity** subscription too (for domain controllers). The
hub may use Azure Firewall, a VPN or ExpressRoute gateway, and Azure Virtual WAN. CloudTyke models the resources and
their rules, but no real packets flow. You can still trace a packet's path with `/_cloudtyke/network/trace` (see
Lesson 03).

## Same idea in AWS and GCP

- **AWS**: shared parts live in their own accounts under an **Infrastructure** or **Security** OU. A **Network**
  account holds the Transit Gateway and Network Firewall. **Log Archive** and **Audit** accounts are the control room
  (CloudTrail, Security Hub). The kit room is **ECR**.
- **GCP**: a `platform` folder holds a **Shared VPC host project** with Cloud NGFW, and a **logging project** with log
  sinks and Security Command Center. The kit room is **Artifact Registry**.
