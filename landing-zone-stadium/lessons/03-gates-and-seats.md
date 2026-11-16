# Lesson 03: Gates and seats

*About 30 minutes. Site: Chapter 2, "Gates, walkways and doors".*

## Why it matters

Every fan must come in through the gates and the bag check. Then they walk to their own stand. A fan in the home end
must not wander into the away end. And nobody should climb in over a back wall.

In the cloud: all traffic goes through the **hub firewall (the bag check)**, every event gets the **same seats (the
baseline network)**, and **door rules (NSGs)** decide who may go where.

## How it works

**Hub and spoke.** The hub is the gates, in `sub-connectivity`. Each event gets a **spoke**: its own VNet (its own
stand). A **peering (a walkway)** joins each spoke to the hub.

```
             internet (the road)
                    │
              ┌─────▼─────┐
              │    hub    │  firewall = bag check, Bastion = staff door
              └─┬───────┬─┘
       walkway  │       │  walkway
          ┌─────▼──┐  ┌─▼──────┐
          │ derby  │  │ summer │   spokes = stands, one per event
          └────────┘  └────────┘
```

**Walkways do not chain.** Peering is **not transitive**. Derby is joined to the hub. Summer is joined to the hub. That
does **not** join derby to summer. If derby needs to reach summer, the traffic must go to the hub firewall, which
decides. That is good: the firewall sees and checks it.

**A sign on every stand: "this way to the bag check".** Each spoke subnet has a **route table** that sends
`0.0.0.0/0` (everywhere) to the hub firewall's private IP. Without it, traffic would skip the check.

**Door rules.** A **network security group (NSG)** is a list of door rules on a subnet. Each rule has a **priority**
(a lower number is checked first), a direction, a source, a destination, a port, and allow or deny. The first rule
that matches wins.

NSGs are **stateful**: if a fan is let in, the steward lets them back out the same way without a new rule. Some door
rules elsewhere are **stateless** (AWS network ACLs, for example): you must write the "in" rule and the "out" rule.

**The same seats for everyone.** `terraform/seats.tf` builds the baseline for every event with `for_each` over
`var.events`: a spoke VNet of the same shape, the same subnets, the same NSGs, the route to the hub firewall, the
peering both ways, private DNS links and RBAC groups. A new event gets the same seats, automatically.

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `terraform/seats.tf`. Find, for one event:
   - the spoke VNet and its subnets
   - the NSG and its rules (note the priorities)
   - the route table with `0.0.0.0/0` to the firewall
   - the two peerings (spoke to hub, hub to spoke)
3. In a toolbox shell (`cd azure-emulator && make shell`), get a token as in Lesson 01, then list the networks in
   `sub-events` with their address ranges:

   ```bash
   S=/subscriptions/5b5c0000-0000-4000-8000-000000000004
   curl -fsS -H "Authorization: Bearer $T" \
     "$M$S/providers/Microsoft.Network/virtualNetworks?api-version=2024-05-01" \
     | jq -r '.value[] | "\(.name)  \(.properties.addressSpace.addressPrefixes | join(","))"'
   ```

4. Pick one address inside the derby spoke and one inside the summer spoke (for example the `.4` address of a
   subnet). Trace a "fan" from one to the other:

   ```bash
   curl -fsS -H "Authorization: Bearer $T" \
     "$M/_cloudtyke/network/trace?from=<derby-ip>&to=<summer-ip>&port=443" | jq
   ```

   Read the hops: which route was chosen, which peering was checked, which NSG rule matched. You should see the
   traffic go to the hub firewall, not straight across.

5. In the site, Chapter 2: trace a fan, fix the broken walkways, and write door rules with priorities.

## Break it

Do these one at a time, and put each back after.

1. **Remove the route.** Comment out the route table association for one spoke subnet and apply. Trace again. The
   fan now skips the bag check. Nothing stops it, and the control room sees nothing. This is the most common hub and
   spoke mistake.
2. **Peer one side only.** A walkway must be built from both ends. Remove the hub-to-spoke peering and trace. The
   walkway is not connected.
3. **Wrong priority.** Add an NSG rule "deny all inbound" at priority `100`, above your "allow 443" rule at `200`. The
   deny wins, because lower numbers are checked first. Swap the numbers to fix it.

## Check yourself

1. Derby is peered to the hub. Summer is peered to the hub. Can derby reach summer directly? Why is that good?
2. What does the `0.0.0.0/0` route on a spoke subnet do?
3. Two NSG rules match a packet: priority 300 allow, priority 150 deny. What happens?
4. What does "stateful" mean for a door rule?
5. Why build seats with `for_each` and not by hand for each event?

## In real Azure

The same model: hub VNet, spoke VNets, peering in both directions, user-defined routes to Azure Firewall, NSGs on
subnets. In real Azure, packets really flow and the firewall really inspects them. CloudTyke checks the configuration
and gives you a hop-by-hop trace. Azure also has **Network Watcher** (IP flow verify, next hop) for the same kind of
question, and **Azure Virtual Network Manager** to build hub and spoke for many VNets at once (see
`azure-emulator/labs/avnm`).

## Same idea in AWS and GCP

- **AWS**: a **Transit Gateway** is the hub. Each account's **VPC** is a spoke. **Security groups** are stateful door
  rules on each server. **Network ACLs** are stateless rules on a subnet. **Network Firewall** is the bag check.
- **GCP**: a **Shared VPC** host project is the hub, and service projects use its subnets. **VPC firewall rules** (or
  Cloud NGFW policies) are stateful, have priorities (lower wins, like Azure) and can target by network tag or
  service account. VPC peering in GCP is not transitive either.
