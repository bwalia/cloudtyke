# CloudTyke Arena: learn landing zones with a stadium

A big stadium is built once. On Saturday it hosts a football match. Next week it hosts a cricket Test. After that, a
concert. The crew changes the pitch fast, but the gates, the seats, the security and the rules never change.

A cloud **landing zone** works the same way. The platform team builds the shared parts once: the network, the
security and the rules. Then any team can host any app (any "event") on it, and swap it quickly. Only the pitch
changes.

This folder teaches that idea with an interactive stadium picture, ten short lessons and a runnable Azure version on
CloudTyke. The AWS and GCP names are a teaching map only. The runnable code is Azure, because CloudTyke emulates Azure.

> CloudTyke Arena, Contoso Pay, Northwind and Fabrikam are made up. The real venues below are only examples of the
> idea, with links to where the facts come from.

## The stadium

```
                       road (the internet)
   ===========================================================
        |  gate A  |              |  gate B  |      staff door
   +----+----------+--------------+----------+--------(Bastion)---+
   |    security ring: turnstiles and bag checks (hub firewall)   |
   |  +--------------------------------------------------------+  |
   |  |  stands and seats (the same baseline for every event)  |  |
   |  |   +------------------------------------------------+   |  |
   |  |   |                                                |   |  |
   |  |   |        THE PITCH (changes per event)           |   |  |
   |  |   |   football  |  cricket  |  concert             |   |  |
   |  |   |                                                |   |  |
   |  |   +------------------------------------------------+   |  |
   |  |  stands and seats                                      |  |
   |  +--------------------------------------------------------+  |
   |  [control room]      [club safe]        [kit room]           |
   +--------------------------------------------------------------+
                                     +---------------------+
       the land: Tenant Root Group   |  training ground    |
                                     |  (ct-sandbox)       |
                                     +---------------------+
```

## The map: stadium part to cloud part

| Stadium | What it means | Azure (CloudTyke names) | AWS | GCP |
|---|---|---|---|---|
| The land and its owner | the top of everything | Tenant Root Group (tenant `7c10d7e4-…0001`) | Organization root | Organization node |
| The stadium shell | shared parts, built once | `ct-platform` management group | Infrastructure / Security OUs | `platform` folder |
| Entry gates, turnstiles, bag checks | one way in, everything checked | `sub-connectivity`: hub VNet, Azure Firewall, Bastion (staff door), private DNS (signs) | Network account: Transit Gateway, Network Firewall | Shared VPC host project, Cloud NGFW |
| Control room and CCTV | watch everything, keep the tapes | `sub-management`: Log Analytics | Log Archive and Audit accounts (CloudTrail, Security Hub) | Logging project, log sinks, Security Command Center |
| Kit room | shared gear for every event | shared Container Registry | ECR | Artifact Registry |
| Seats and stands | the same standard setup for every event | the landing zone baseline: spoke VNet shape, subnets, NSGs, route to the hub firewall, peering, DNS links, RBAC groups | account baseline (VPC, attachments, roles) | project baseline (Shared VPC service project, IAM) |
| Stadium safety code | the rules, same for all events | Azure Policy initiative assigned at `ct-landingzones` | SCPs and Control Tower controls | Organization Policy constraints |
| Stewards and wristbands | who can go where | Entra ID groups and Azure RBAC | IAM Identity Center, permission sets | Cloud Identity and IAM |
| The pitch | where the event happens | `ct-landingzones` management group | Workloads OU | `landing-zones` folder |
| One event | one app team's space | a landing zone (subscription, here `sub-events`) | an account | a project |
| Booking an event | get a new ready space fast | subscription vending | Account Factory (AFT) | Project Factory |
| Changeover crew | swaps the pitch between events | CI/CD pipeline with GitHub OIDC (no secrets) | CodePipeline / GitHub OIDC | Cloud Build / Workload Identity Federation |
| Training ground | safe place to practise, looser rules | `ct-sandbox` and `sub-sandbox` | Sandbox OU | `sandbox` folder |

## Open the site

The site is plain HTML, CSS and JavaScript. There is no build step.

- **Double-click** `site/index.html`. It works offline, straight from disk.
- Or **serve the folder**, for example `python3 -m http.server -d landing-zone-stadium 8080`, then open
  <http://localhost:8080/site/>.

Start with Chapter 1, "Build the stadium". Pick your venue (football, cricket, concert or film studio) and your
cloud (Azure, AWS or GCP) on the landing page. Your progress is saved in your browser.

## The lessons

Each lesson takes 15 to 30 minutes. They follow the [CloudTyke Academy](../azure-emulator/academy/README.md)
format: Why it matters, How it works, Do it, Break it, Check yourself, In real Azure, and Same idea in AWS and GCP.

1. [The land](lessons/01-the-land.md)
2. [The stadium shell](lessons/02-the-stadium-shell.md)
3. [Gates and seats](lessons/03-gates-and-seats.md)
4. [The safety code](lessons/04-the-safety-code.md)
5. [The pitch and event kits](lessons/05-the-pitch-and-event-kits.md)
6. [Swap the event](lessons/06-swap-the-event.md)
7. [Kubernetes is the team](lessons/07-kubernetes-the-team.md)
8. [Bin packing: the best team](lessons/08-bin-packing-best-team.md)
9. [Any show: native, Docker, Kubernetes and storage](lessons/09-any-show-native-docker-kubernetes-storage.md)
10. [Overflow car parks and bridges](lessons/10-overflow-car-parks-and-bridges.md)

## Run the Azure version on CloudTyke

You need Docker. From the repo root:

```bash
bash landing-zone-stadium/scripts/up.sh   # start the emulator with the stadium seed, then tofu init + apply
bash landing-zone-stadium/verify.sh       # PASS / FAIL checks
```

> **Heads up:** `up.sh` re-seeds the emulator (`POST /_cloudtyke/reset`), which wipes everything already in it,
> for example a Contoso Pay scenario. Run `KEEP_STATE=1 bash landing-zone-stadium/scripts/up.sh` to keep what is
> there and only add `sub-events`.

`up.sh` starts the emulator with `docker-compose.stadium.yml`. That file mounts `seed.yaml`, which is the default
seed plus one more subscription, `sub-events`, under `ct-landingzones`. Then it runs OpenTofu in the toolbox on
`terraform/`:

| File | Stadium part |
|---|---|
| `terraform/platform.tf` | gates (hub network, firewall, Bastion, private DNS, shared registry) and the control room |
| `terraform/safety-code.tf` | the safety code: one policy initiative at `ct-landingzones` |
| `terraform/seats.tf` | the same seats for every event (`for_each` over `var.events`) |
| `terraform/pitch.tf` | the pitch, picked from a catalogue of event kits |
| `terraform/variables.tf` | the events: `derby` (football) and `summer` (cricket) |

Change `derby` from `football` to `concert` and run a plan. Only the derby pitch changes. `verify.sh` checks that.

Open the dashboard (`cd azure-emulator && make dashboard`, then <http://localhost:4310>) and look at the Resources
page. The names match the picture.

## Real venues and their sources

The site and lessons use a few true stories. These are the sources. The facts are written in our own words.

- Colosseum underground, lifts and trapdoors: <https://www.througheternity.com/travel-guide/colosseum-underground-a-history>
- Colosseum flooding, hypogeum and the sailors' awning: <https://www.thecollector.com/ancient-roman-engineering-ultimate-death-arena/>
- Drop-in pitches at the MCG and other grounds: <https://www.sportskeeda.com/cricket/drop-in-pitches-pros-and-cons>
- New York 2024 drop-in pitches grown in Florida: <https://thefederal.com/cricket/drop-in-pitch-for-india-vs-pakistan-t20-world-cup-2024-match-all-you-need-to-know-124831>
- Retractable pitches (Tottenham, Veltins-Arena, Sapporo Dome, GelreDome): <https://ingenia.org.uk/articles/tottenham-hotspur-stadium-the-football-pitch-in-three-pieces/>
- First retractable grass pitch (GelreDome, 1998): <https://www.guinnessworldrecords.com/world-records/113967-first-retractable-grass-pitch>
- Arena changeovers over the ice: <https://www.fox5dc.com/news/behind-the-scenes-look-at-how-capital-one-arena-changes-over-for-events>
- Ramoji Film City (Guinness record, sound stages, permanent sets): <https://www.guinnessworldrecords.com/world-records/largest-film-studio>
- Ramoji Film City, "walk in with a script" and sets left standing: <https://scroll.in/reel/860202/location-scouting-everything-is-about-show-business-at-ramoji-film-city-not-just-for-filmmakers>
- Hengdian World Studios, Forbidden City copy and many shoots at once: <https://www.npr.org/2007/08/13/12741465/ambition-and-history-meet-in-chinas-hollywood>
- Morbi bridge investigation findings: <https://www.outlookindia.com/national/morbi-bridge-collapse-broken-wires-welded-suspenders-behind-tragedy-reveals-sit-report-news-263524>
- Morbi bridge reopened without a fitness certificate: <https://www.deccanherald.com/amp/story/india%2Fcollapsed-morbi-suspension-bridge-had-reopened-4-days-back-after-7-month-long-repair-work-lacked-civic-bodys-fitness-certificate-1158001.html>
- Millennium Bridge wobble and dampers: <https://hidden-london.com/?p=16068>
- Silver Bridge and redundancy: <https://wvpublic.org/story/government/silver-bridge-anniversary-renews-commitment-to-safety-standards/>
- Hyatt Regency walkways (NIST report): <https://nvlpubs.nist.gov/nistpubs/Legacy/IR/nbsir82-2465.pdf>
- Tacoma Narrows flutter: <https://practical.engineering/blog/2019/3/9/why-the-tacoma-narrows-bridge-collapsed>
- I-35W NTSB findings: <https://roadsbridges.com/ntsb-releases-report-i-35w-bridge-collapse>
