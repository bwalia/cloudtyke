# Lesson 10: Overflow car parks and bridges

*About 30 minutes. Site: Chapter 8, "Match day rush".*

## Why it matters

On match day, thousands of fans arrive in the same hour. If the car park is full, they need somewhere else to go. If
everyone crosses one footbridge at once, the bridge must be strong enough, or someone must stop the crowd at the
entrance.

Clouds have match days too: a ticket sale opens, a goal is scored, a region goes down. The landing zone gives every
event the tools to stay up. This lesson is about using them.

## How it works

Three pictures: the **overflow car park**, the **twin stadium in another city** and the **bridge with a weight
limit**.

| Stadium and city | Cloud idea | Azure | AWS | GCP |
|---|---|---|---|---|
| Stands with their own power and entrances, inside one stadium | availability zones | availability zones | Availability Zones | zones |
| A twin stadium in another city | regions | regions | Regions | regions |
| An overflow car park that opens only when the main one is full or closed | active-passive failover (warm standby) | Front Door or Traffic Manager priority routing | Route 53 failover routing | Cloud Load Balancing / Cloud DNS failover |
| Two car parks open all the time, fans sent to the nearest | active-active | Front Door latency routing | Route 53 latency routing | global external load balancer |
| Stewards radioing "car park A is full / closed" | health probes | health probes | health checks | health checks |
| Motorway signs that change to "use car park B" | DNS and global routing | Front Door, Traffic Manager | Route 53 | Cloud DNS, global load balancer |
| How long until the overflow car park is open | RTO (recovery time) | | | |
| How many ticket sales you lose in the switch | RPO (data loss) | geo-redundant storage, read replicas, Cosmos DB multi-region | S3 replication, Aurora Global Database | dual-region buckets, Cloud SQL replicas, Spanner |
| Opening more turnstiles when the queue grows | autoscaling | VM Scale Sets, AKS autoscaler, Container Apps scaling | Auto Scaling, Karpenter | managed instance groups, GKE autoscaler, Cloud Run |
| Opening extra gates **before** kick-off, not after | scheduled or predictive scaling | | | |
| Barriers, timed entry, "wait here" queues | rate limits, virtual waiting rooms, queues that level the load | API Management limits, Service Bus queues | API Gateway throttling, SQS | Apigee quotas, Pub/Sub |
| Stewards stop entry when a stand is full | load shedding (`429` / `503`) | | | |
| Closing a gate when the crowd behind it is crushing | circuit breaker | | | |
| Letting fans leave stand by stand, not all at once | backoff with jitter (no thundering herd) | | | |
| Fire drills | failover drills and chaos tests (CloudTyke `tyke azure chaos`) | Chaos Studio | Fault Injection Service | fault testing on GKE / Cloud Run |

Three key lessons:

- **The overflow car park must be big enough.** If the second region is too small, failover just moves the crowd to a
  place that also breaks. Reserve the capacity and test it.
- **Don't let everyone rush at once.** After a failover or an outage, every client retries at the same moment. Spread
  them out with backoff and jitter, queues and rate limits.
- **Weight limits must be enforced at the entrance**, not just written on a sign.

**Backoff with jitter**, in plain words: when you get "too busy, try later" (`429` or `503`), wait a little before you
try again. If it fails again, wait longer (1, 2, 4, 8 seconds). And add a random part, so a thousand clients do not
all come back at the very same second. If the server says `Retry-After: 2`, wait at least 2 seconds.

### What bridges teach us

Real bridges have failed, and real people died. These short notes are told with respect. Each one taught engineers
something that cloud teams need too.

| Bridge | What went wrong | Cloud lesson |
|---|---|---|
| Morbi, India (2022) | Thousands of tickets were sold for an old suspension bridge just after a renovation. It reopened without a fitness certificate or a load test, a heavier deck had been added, and the cables were corroded. About 300 people were on it when it fell. | Limit how many get in at the door (admission control). Load-test every change before go-live. No release without sign-off. |
| Millennium Bridge, London (2000) | Did not fall, but it swayed on opening day because walkers fell into step with the sway, which made it worse. It closed and was fixed with dampers. | Clients that act in sync (retry storms) shake the system. Add dampers: jitter, backoff and rate limits. |
| Silver Bridge, USA (1967) | One small crack in one steel link brought down the whole bridge, because each link had only two bars and no backup. | No single point of failure. Use many zones, replicas and paths, like a cable made of many wires. |
| Hyatt Regency walkways, USA (1981) | A "simple" change during building doubled the load on a connection. Nobody re-checked the maths. | Review every change, even small ones. Use pull requests, plans and policy checks. |
| Tacoma Narrows, USA (1940) | The bridge was strong enough for its weight, but wind made it twist more and more until it broke. | Test for real-world behaviour, not just size. Watch for feedback loops (autoscaler flapping, retry storms). |
| I-35W, Minneapolis (2007) | Some steel plates were designed at half the right thickness. The bridge stood for 40 years until extra weight was added over time, plus building materials on the day. | Hidden weak spots wait for a busy day. Re-check capacity as things grow and keep monitoring. |

The sources are listed in the [README](../README.md#real-venues-and-their-sources).

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. In the site, open Chapter 8, "Match day rush". First press play with every control **off**. Watch the bridge load
   go red. Then turn on, one at a time: turnstile autoscaling, the waiting queue, the weight limit at the bridge
   entrance and jittered exits. Watch the three numbers: fans waiting, fans turned away and bridge load.
3. Press "region down" to close the main car park. Does the overflow car park cope?
4. Open a toolbox shell (`cd azure-emulator && make shell`) and get a token as in Lesson 01. Write a tiny "fan
   app" that reads the event's resource groups 30 times, with **no** retries:

   ```bash
   S=/subscriptions/5b5c0000-0000-4000-8000-000000000004
   call() { curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $T" \
     "$M$S/resourceGroups?api-version=2024-11-01"; }
   for i in $(seq 1 30); do call; done | sort | uniq -c
   ```

   All `200`. The stadium is calm.

## Break it

Now make it a match day. Turn on chaos: 40% of requests are throttled (`429` with `Retry-After`) and 10% fail with a
`5xx`. Either use the `tyke` tool on your laptop:

```bash
cd azure-emulator && make build
bin/tyke azure chaos set --throttle 0.4 --errors 0.1
bin/tyke azure chaos get
```

or, from the toolbox shell, call the admin API directly:

```bash
curl -fsS -X PUT -H 'content-type: application/json' "$M/_cloudtyke/chaos" \
  -d '{"throttleRate":0.4,"errorRate":0.1}'
```

Run the fan app again:

```bash
for i in $(seq 1 30); do call; done | sort | uniq -c
```

Now many calls fail. Imagine every client in the world retrying **at once, straight away**. That is the Millennium
Bridge: everyone in step, and the sway gets worse.

**Fix it with backoff and jitter.** Replace `call` with a polite version:

```bash
polite() {
  local try=0 code wait
  while :; do
    code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $T" \
      "$M$S/resourceGroups?api-version=2024-11-01")
    case $code in 429|5??) ;; *) echo "$code after $try retries"; return ;; esac
    try=$((try + 1))
    [ $try -gt 6 ] && { echo "gave up ($code)"; return; }
    wait=$(( (1 << try) ))                    # 2, 4, 8 ... seconds: backoff
    wait=$(( RANDOM % wait + 1 ))             # a random part of it: jitter
    sleep "$wait"
  done
}
for i in $(seq 1 10); do polite; done
```

Almost every call now ends in `200`, after a few spread-out retries. Nobody rushes the gate at the same second.
(Real clients also read the `Retry-After` header and wait at least that long. The Azure SDKs and the Terraform
provider already do backoff for you.)

Turn chaos off when you are done:

```bash
bin/tyke azure chaos clear      # or: curl -fsS -X DELETE "$M/_cloudtyke/chaos"
```

## Check yourself

1. What is the difference between active-passive and active-active?
2. Your second region has half the capacity of the first. What happens on failover? What should you do?
3. What do RTO and RPO mean, in car park words?
4. Why add a random part (jitter) to the wait, and not just double it?
5. Which bridge story matches a retry storm? Which matches a single point of failure?
6. Why is a weight limit "at the entrance" better than a sign?

## In real Azure

Azure has **availability zones** in most regions, and paired **regions**. **Front Door** and **Traffic Manager** route
fans to healthy endpoints using health probes. **API Management** sets rate limits. **Service Bus** queues level the
load. **Azure Chaos Studio** runs real fault experiments, like a fire drill. Azure Resource Manager itself throttles
busy callers with `429` and `Retry-After`, so your scripts and pipelines need backoff too. CloudTyke's chaos switch
gives you those same `429` and `5xx` answers on purpose, so you can practise safely.

## Same idea in AWS and GCP

- **AWS**: Availability Zones and Regions, **Route 53** failover and latency routing, **API Gateway** throttling,
  **SQS** queues, **Auto Scaling** and Karpenter, and **Fault Injection Service** for drills. The AWS SDKs retry with
  backoff and jitter by default.
- **GCP**: zones and regions, the **global external load balancer** and **Cloud DNS** routing, **Apigee** quotas,
  **Pub/Sub** queues, managed instance groups and the GKE autoscaler. Google's client libraries also retry with
  exponential backoff.
