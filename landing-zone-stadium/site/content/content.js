/* CloudTyke Arena: all teaching text lives here, in one data object (window.LZS).
   Edit or translate this file. Code in js/ only reads it.
   Cloud arrays are always [Azure, AWS, GCP]. Azure names match the CloudTyke seed. */
window.LZS = {
  ui: {
    brand: "CloudTyke Arena",
    tagline: "Learn cloud landing zones with a stadium.",
    oneLine: "A landing zone is the stadium you build once: gates, seats, security and rules. Every app is an event you can host on it, and swap fast.",
    clouds: ["Azure", "AWS", "GCP"],
    cloudKeys: ["azure", "aws", "gcp"],
    teachingOnly: "AWS and GCP are a teaching map only. The runnable code is Azure, because CloudTyke emulates Azure.",
    perm: "Never rebuilt",
    change: "Changes per event",
    unchanged: "unchanged",
    showLabels: "Show all labels",
    panelCloud: "In the cloud",
    panelCt: "CloudTyke name",
    panelWhy: "Why it matters",
    tapAgain: "Tap again for more",
    next: "Next chapter",
    prev: "Previous chapter",
    home: "All chapters",
    check: "Check yourself",
    checkIntro: "Three quick questions. Get all three right to tick this chapter.",
    checkDone: "All right. Chapter ticked.",
    checkTry: "Not yet. Read the hints and try again.",
    reset: "Reset my progress"
  },

  /* --- Every part of every drawing. id -> info ---------------------------------- */
  parts: {
    land: { perm: true, related: ["shell", "training"], name: "The land and its owner",
      what: "Everything sits on one piece of land with one owner. Nothing is above it.",
      c: ["Tenant Root Group (tenant 7c10d7e4-…0001)", "Organization root", "Organization node"],
      l: ["Tenant Root Group", "Organization root", "Organization node"],
      ct: "Tenant Root Group", why: "Rules and access set here reach every part below. Touch it rarely and carefully." },
    road: { perm: true, related: ["gates"], name: "The public road",
      what: "Anyone can walk along the road. Not everyone may come in.",
      c: ["The internet", "The internet", "The internet"], l: ["Internet", "Internet", "Internet"],
      ct: "(outside the tenant)", why: "Treat everything on the road as a stranger until the gates check it." },
    shell: { perm: true, related: ["gates", "control", "safe", "kitroom", "firewall"], name: "The stadium shell",
      what: "The walls and roof. The shared parts live here, built once by the platform team.",
      c: ["ct-platform management group", "Infrastructure and Security OUs", "platform folder"],
      l: ["ct-platform", "Infrastructure OU", "platform folder"],
      ct: "ct-platform", why: "Shared parts are built once and looked after by one team, so every event gets the same quality." },
    gates: { perm: true, related: ["firewall", "stands", "road"], name: "Entry gates",
      what: "Every fan comes in through the same gates. There is one way in.",
      c: ["Hub VNet in sub-connectivity", "Network account with Transit Gateway", "Shared VPC host project"],
      l: ["Hub VNet", "Transit Gateway", "Shared VPC host"],
      ct: "sub-connectivity", why: "One way in means one place to check, log and block. Side doors are how break-ins happen." },
    firewall: { perm: true, related: ["gates", "stands"], name: "Turnstiles and bag check",
      what: "A ring of checks. Every bag is looked at before a fan reaches a seat.",
      c: ["Azure Firewall in sub-connectivity", "AWS Network Firewall", "Cloud NGFW"],
      l: ["Azure Firewall", "Network Firewall", "Cloud NGFW"],
      ct: "sub-connectivity", why: "All traffic is routed through the bag check (the hub firewall), so bad traffic stops in one place." },
    staffdoor: { perm: true, related: ["gates", "stewards"], name: "Staff door",
      what: "A locked door for staff only. You show your pass, and every visit is written down.",
      c: ["Azure Bastion in sub-connectivity", "Systems Manager Session Manager", "Identity-Aware Proxy (IAP)"],
      l: ["Bastion", "Session Manager", "IAP"],
      ct: "sub-connectivity", why: "Admins reach machines without public IPs or open SSH ports." },
    stands: { perm: true, related: ["gates", "firewall", "pitch"], name: "Seats and stands",
      what: "The same standard seating for every event. Same rows, same exits, same signs.",
      c: ["Landing zone baseline: spoke VNet, subnets, NSGs, route to the hub firewall, peering, DNS links, RBAC groups", "Account baseline: VPC, attachments, roles", "Project baseline: Shared VPC service project, IAM"],
      l: ["Spoke baseline", "Account baseline", "Project baseline"],
      ct: "seats.tf (for_each over events)", why: "Every event gets the same safe setup with no extra work. Nobody hand-builds seats." },
    pitch: { perm: false, related: ["stands", "crew", "boxoffice"], name: "The pitch",
      what: "Where the event happens. This is the only part that changes.",
      c: ["ct-landingzones management group, with one subscription per event (here sub-events)", "Workloads OU, one account per event", "landing-zones folder, one project per event"],
      l: ["ct-landingzones", "Workloads OU", "landing-zones folder"],
      ct: "ct-landingzones / sub-events", why: "App teams build fast on the pitch while the stadium keeps them safe." },
    control: { perm: true, related: ["stands", "pitch", "gates"], name: "Control room and CCTV",
      what: "Screens that watch every gate and stand. The tapes are kept.",
      c: ["Log Analytics in sub-management", "Log Archive and Audit accounts (CloudTrail, Security Hub)", "Logging project, log sinks, Security Command Center"],
      l: ["Log Analytics", "Log Archive account", "Logging project"],
      ct: "sub-management", why: "You can only fix what you can see. Logs live outside the events, so an event cannot wipe them." },
    safe: { perm: true, related: ["pitch", "stewards"], name: "The club safe",
      what: "A strong room for the trophies, the players' wallets and the private files.",
      c: ["Key Vault", "Secrets Manager + KMS", "Secret Manager + Cloud KMS"],
      l: ["Key Vault", "Secrets Manager", "Secret Manager"],
      ct: "kv-<event> per event, one safe design", why: "Secrets never sit in code. Each app opens only its own locker." },
    kitroom: { perm: true, related: ["pitch", "crew"], name: "Kit room",
      what: "Shared gear for every event, checked and ready to use.",
      c: ["Shared Azure Container Registry", "Amazon ECR", "Artifact Registry"],
      l: ["Container Registry", "ECR", "Artifact Registry"],
      ct: "shared ACR in sub-connectivity", why: "Every event pulls tested images from one trusted place." },
    rules: { perm: true, related: ["pitch", "training", "stands"], name: "Stadium safety code",
      what: "The rules on the board by the gate. The same for every event.",
      c: ["Azure Policy initiative assigned at ct-landingzones", "SCPs and Control Tower controls", "Organization Policy constraints"],
      l: ["Azure Policy", "SCPs", "Org Policy"],
      ct: "policy initiative at ct-landingzones", why: "Rules are checked by the system, not by hope. A bad request is refused before it is built." },
    stewards: { perm: true, related: ["gates", "staffdoor", "safe"], name: "Stewards and wristbands",
      what: "Stewards check wristbands. Your band says where you may go.",
      c: ["Entra ID groups and Azure RBAC", "IAM Identity Center and permission sets", "Cloud Identity and IAM"],
      l: ["Entra ID + RBAC", "Identity Center", "Cloud IAM"],
      ct: "Entra ID groups", why: "People get access by group and role, not one by one. Easy to give, easy to take away." },
    boxoffice: { perm: true, related: ["pitch", "crew"], name: "Box office: book an event",
      what: "Book a date and you get a ready space with seats, gates and rules already in place.",
      c: ["Subscription vending", "Account Factory (AFT)", "Project Factory"],
      l: ["Subscription vending", "Account Factory", "Project Factory"],
      ct: "seed.yaml adds sub-events", why: "A new team gets a safe space in minutes, not weeks." },
    crew: { perm: true, related: ["pitch", "kitroom"], name: "Changeover crew",
      what: "The crew that swaps the pitch between events, fast, from a checklist.",
      c: ["CI/CD pipeline with GitHub OIDC (no secrets)", "CodePipeline or GitHub OIDC", "Cloud Build or Workload Identity Federation"],
      l: ["Pipeline + OIDC", "CodePipeline", "Cloud Build"],
      ct: "tofu apply from a pipeline", why: "A script does the same change the same way every time. No stored passwords." },
    training: { perm: true, related: ["land", "rules"], name: "Training ground",
      what: "A safe place to practise next to the stadium. The rules are looser here.",
      c: ["ct-sandbox and sub-sandbox", "Sandbox OU", "sandbox folder"],
      l: ["ct-sandbox", "Sandbox OU", "sandbox folder"],
      ct: "ct-sandbox / sub-sandbox", why: "People learn by trying. Mistakes here never touch a real event." },

    /* hub and spoke (chapter 2) */
    "n-hub": { perm: true, related: ["n-peer"], name: "The concourse (hub)", what: "The big walkway behind the gates. Every path starts here.",
      c: ["Hub VNet", "Transit Gateway or inspection VPC", "Shared VPC host network"], ct: "sub-connectivity", why: "One central place to inspect and route." },
    "n-peer": { perm: true, related: ["n-hub", "n-spoke"], name: "Walkway (peering)", what: "A walkway joins the concourse to one stand. It must be open at both ends.",
      c: ["VNet peering (both directions)", "Transit Gateway attachment", "Shared VPC (or VPC peering)"], ct: "azurerm_virtual_network_peering x2", why: "Peering is not transitive. Two stands joined to the concourse are not joined to each other." },
    "n-spoke": { perm: false, related: ["n-peer", "n-nsg"], name: "An event's stand (spoke)", what: "Each event has its own stand with its own seats.",
      c: ["Spoke VNet in the event subscription", "Workload VPC in the event account", "Service project subnet"], ct: "vnet-<event>", why: "Events are kept apart. One event cannot wander into another." },
    "n-route": { perm: true, related: ["n-hub", "firewall"], name: "Signpost to the bag check (route)", what: "A sign that says: to go anywhere else, walk via the bag check.",
      c: ["Route table: 0.0.0.0/0 to the firewall", "VPC route table to the inspection VPC", "Custom route to the NGFW"], ct: "rt-<event>", why: "Without the sign, traffic tries to skip the bag check, or gets lost." },
    "n-nsg": { perm: true, related: ["n-spoke"], name: "Door rules", what: "A list on each door: who may pass, checked from the top.",
      c: ["Network security group (NSG)", "Security group, plus network ACL", "VPC firewall rules"], ct: "nsg-<event>", why: "Even inside the stadium, each door only lets in the right people." },

    /* the club safe (chapter 4) */
    "v-room": { perm: true, related: ["v-secrets", "v-keys", "v-certs"], name: "The strong room", what: "One strong room. Never rebuilt. Each event gets its own lockers inside.",
      c: ["Key Vault", "Secrets Manager + KMS", "Secret Manager + Cloud KMS"], ct: "kv-<event>", why: "One well-guarded place beats secrets scattered everywhere." },
    "v-secrets": { perm: false, related: ["v-identity"], name: "Players' lockers", what: "Each player keeps a wallet in a locker. Passwords, API keys and connection strings live here.",
      c: ["Key Vault secrets", "Secrets Manager secrets", "Secret Manager secrets"], ct: "secret: db-password", why: "Apps read secrets at run time. Nothing secret is in the code." },
    "v-keys": { perm: true, related: ["v-purge"], name: "Key to the trophy cabinet", what: "The key that locks and unlocks the most precious things.",
      c: ["Key Vault keys / Managed HSM", "KMS keys", "Cloud KMS keys"], ct: "customer-managed key", why: "Encryption keys protect data at rest. You decide who may use them." },
    "v-certs": { perm: true, related: ["v-room"], name: "The club's official seal", what: "The seal proves a letter really came from the club.",
      c: ["Key Vault certificates", "AWS Certificate Manager (ACM)", "Certificate Manager"], ct: "certificate: tickets-tls", why: "Certificates prove who a site is, and expire. The vault renews them for you." },
    "v-identity": { perm: false, related: ["v-secrets"], name: "A wristband, not a key on a string", what: "The app proves who it is, like a wristband. No password needed.",
      c: ["Managed identity + Azure RBAC", "IAM role + key policy", "Service account + IAM"], ct: "id-<event>-app", why: "No password to leak, nothing to rotate by hand." },
    "v-tape": { perm: false, related: ["v-identity"], name: "A key taped to the door", what: "A password in code or a settings file. Never do this.",
      c: ["Password in appsettings.json", "Access key in a .env file", "Key file in the repo"], ct: "(found in the config)", why: "Anyone who sees the code, a log or a backup now has the key." },
    "v-purge": { perm: true, related: ["v-keys"], name: "The trophies can't be thrown away", what: "Even by mistake, nobody can bin the trophies for good.",
      c: ["Soft delete and purge protection", "Recovery window and key deletion wait", "Secret versions and key destroy delay"], ct: "purge_protection_enabled = true", why: "A slip or an attacker cannot destroy keys for ever." },
    "v-book": { perm: true, related: ["control"], name: "The safe's visitor book", what: "Every visit to the safe is written down and sent to the control room.",
      c: ["Key Vault diagnostic logs", "AWS CloudTrail", "Cloud Audit Logs"], ct: "diagnostic setting to Log Analytics", why: "You can answer: who opened which locker, and when?" },

    /* the control room (chapter 5) */
    "o-traces": { perm: true, related: ["o-otel"], name: "A camera that follows one fan", what: "Follows one fan from gate to seat and times every step.",
      c: ["Application Insights", "AWS X-Ray", "Cloud Trace"], why: "Shows exactly which step was slow for one request." },
    "o-metrics": { perm: true, related: ["o-alerts", "o-dash"], name: "Turnstile counters", what: "Numbers over time: fans per minute, queue length.",
      c: ["Azure Monitor metrics", "CloudWatch metrics", "Cloud Monitoring"], why: "Cheap to keep, great for trends and alerts." },
    "o-logs": { perm: true, related: ["o-tapes"], name: "The steward's notebook", what: "Written notes of what happened, line by line.",
      c: ["Log Analytics", "CloudWatch Logs", "Cloud Logging"], why: "The detail you need when you know where to look." },
    "o-otel": { perm: true, related: ["o-collector", "o-traces"], name: "The standard camera fitting", what: "Any camera fits any control room, because the fitting is standard.",
      c: ["Azure Monitor OpenTelemetry distro", "AWS Distro for OpenTelemetry", "OpenTelemetry on Google Cloud"], ct: "OpenTelemetry SDK", why: "Add it once. Change the control room later without touching the app." },
    "o-collector": { perm: true, related: ["o-otel"], name: "The video desk", what: "Collects every feed and sends each to the right screens.",
      c: ["OpenTelemetry Collector", "OpenTelemetry Collector", "OpenTelemetry Collector"], ct: "otel-collector", why: "One place to filter, add labels and send data on." },
    "o-alerts": { perm: true, related: ["o-metrics"], name: "Stewards' radios", what: "\"Gate 3 queue too long!\" A call when a number goes wrong.",
      c: ["Azure Monitor alerts", "CloudWatch alarms", "Cloud Monitoring alerting"], why: "People hear about problems before fans complain." },
    "o-dash": { perm: true, related: ["o-metrics"], name: "The big screens", what: "The main numbers on the wall, at a glance.",
      c: ["Workbooks, Managed Grafana", "CloudWatch dashboards, Managed Grafana", "Cloud Monitoring dashboards"], why: "A good screen answers one question fast." },
    "o-security": { perm: true, related: ["o-logs"], name: "Security cameras on the doors", what: "Cameras that look for break-ins, not for queues.",
      c: ["Defender for Cloud, Sentinel", "GuardDuty, Security Hub", "Security Command Center"], why: "Spots attacks and risky settings across every event." },
    "o-tapes": { perm: true, related: ["o-logs"], name: "Tapes kept after the event leaves", what: "The recordings stay after the event has gone home.",
      c: ["Central Log Analytics retention in sub-management", "Log Archive account", "Central log bucket via log sinks"], ct: "sub-management", why: "You can still investigate next week, even if the event was deleted." },

    /* how is the show built (chapter 6) */
    "b-vm": { perm: false, related: ["s-disk"], name: "A set built by hand", what: "Strong, but slow to change. Built on the stage, piece by piece.",
      c: ["Virtual Machines, VM Scale Sets", "EC2, Auto Scaling groups", "Compute Engine, managed instance groups"], why: "Good for old apps that need a whole machine. Slow to swap." },
    "b-docker": { perm: false, related: ["kitroom"], name: "Flat-pack set pieces", what: "Made in the workshop, the same on every stage.",
      c: ["Container Apps, App Service for containers", "ECS with Fargate, App Runner", "Cloud Run"], why: "Build once, run anywhere the same. Fast to start and stop." },
    "b-k8s": { perm: false, related: ["k-control"], name: "The floor manager and many crews", what: "Runs many flat-pack pieces and crews on many stages.",
      c: ["AKS", "EKS", "GKE"], ct: "aks-<event>", why: "Best when you run lots of containers that must scale and heal." },
    "s-object": { perm: true, related: ["b-k8s"], name: "Prop warehouse and film vault", what: "Photos, video, replays and backups. Huge, cheap, kept safe.",
      l: ["Blob Storage", "Amazon S3", "Cloud Storage"], c: ["Blob Storage", "Amazon S3", "Cloud Storage"], ct: "st<event>", why: "Data lives here, not in the containers, so it outlasts them." },
    "s-files": { perm: true, related: ["b-vm"], name: "The shared script library", what: "Files that many people open at once.",
      l: ["Azure Files", "Amazon EFS", "Filestore"], c: ["Azure Files", "Amazon EFS", "Filestore"], why: "Many machines can read and write the same files." },
    "s-disk": { perm: false, related: ["b-vm", "b-k8s"], name: "Each set's lock-up cupboard", what: "A cupboard that belongs to one set only.",
      l: ["Managed Disks", "Amazon EBS", "Persistent Disk"], c: ["Managed Disks, PVCs on AKS", "EBS, PVCs on EKS", "Persistent Disk, PVCs on GKE"], why: "Fast storage for one machine or one pod." },
    "s-db": { perm: true, related: ["s-object"], name: "Continuity notes and score books", what: "The records of every match, kept in order and easy to search.",
      l: ["PostgreSQL", "RDS", "Cloud SQL"], c: ["PostgreSQL flexible server, Cosmos DB", "RDS, DynamoDB", "Cloud SQL, Firestore"], ct: "psql-<event>", why: "Stateful data must stay when pods come and go." },

    /* Kubernetes is the team (chapter 7). Names come from venues.js. */
    "k-control": { perm: false, name: "", what: "", c: ["AKS control plane", "EKS control plane", "GKE control plane"], why: "Keeps the real state matching the plan." },
    "k-manifest": { perm: false, name: "", what: "", c: ["Manifest (YAML): the desired state", "Manifest (YAML): the desired state", "Manifest (YAML): the desired state"], why: "You write what you want. Kubernetes makes it so." },
    "k-scheduler": { perm: false, name: "", what: "", c: ["kube-scheduler places pods on nodes", "kube-scheduler places pods on nodes", "kube-scheduler places pods on nodes"], why: "Good placement saves money and keeps you safe." },
    "k-pod": { perm: false, name: "", what: "", c: ["Pods", "Pods", "Pods"], why: "The smallest unit that runs. Replaceable at any time." },
    "k-node": { perm: false, name: "", what: "", c: ["Nodes in an AKS node pool", "Nodes in an EKS node group", "Nodes in a GKE node pool"], why: "The machines that pods run on." },
    "k-deploy": { perm: false, name: "", what: "", c: ["Deployment with replicas", "Deployment with replicas", "Deployment with replicas"], why: "Say how many copies you want. Kubernetes keeps that many." },
    "k-heal": { perm: false, name: "", what: "", c: ["Self-healing: a failed pod is replaced", "Self-healing: a failed pod is replaced", "Self-healing: a failed pod is replaced"], why: "Failures happen. The team carries on." },
    "k-bench": { perm: false, name: "", what: "", c: ["Spare capacity and pending pods", "Spare capacity and pending pods", "Spare capacity and pending pods"], why: "Pending pods wait for room. Spare room costs money." },
    "k-kubelet": { perm: false, name: "", what: "", c: ["The kubelet on each node", "The kubelet on each node", "The kubelet on each node"], why: "Reports what is really running." },
    "k-referee": { perm: false, name: "", what: "", c: ["Admission control: Azure Policy for AKS (Gatekeeper)", "Admission control: Gatekeeper or Kyverno", "Admission control: Policy Controller (Gatekeeper)"], why: "Illegal play is stopped before it starts." },
    "k-registry": { perm: false, name: "", what: "", c: ["Azure Container Registry", "Amazon ECR", "Artifact Registry"], why: "Every pod pulls its kit from one trusted place." },
    "k-autoscale": { perm: false, name: "", what: "", c: ["HPA and the cluster autoscaler", "HPA and Karpenter or Cluster Autoscaler", "HPA and the GKE autoscaler"], why: "More players when it is busy, fewer when it is quiet." },

    /* match day rush (chapter 8) */
    "r-zones": { perm: true, name: "Stands with their own power", what: "Each stand has its own power and its own entrances.", c: ["Availability zones", "Availability Zones", "Zones"], why: "One stand can fail and the match goes on." },
    "r-main": { perm: true, related: ["r-overflow", "r-dns"], name: "Main car park (main region)", what: "Where most fans park. When it fills or closes, the signs change.", c: ["Primary region (UK South)", "Primary Region", "Primary region"], why: "Your normal home." },
    "r-overflow": { perm: true, related: ["r-main", "r-dns"], name: "Overflow car park (second region)", what: "Opens only when the main one is full or closed.", c: ["Front Door or Traffic Manager priority routing to UK West", "Route 53 failover routing", "Cloud Load Balancing / Cloud DNS failover"], why: "It must be big enough, or failover just moves the crowd to a place that also breaks." },
    "r-dns": { perm: true, related: ["r-main", "r-overflow"], name: "Motorway signs", what: "Signs that change to \"use car park B\".", c: ["Front Door, Traffic Manager", "Route 53", "Cloud DNS, global load balancer"], why: "Fans are sent to the place that is open." },
    "r-probes": { perm: true, related: ["r-dns"], name: "Stewards' radios: car park full", what: "Stewards report \"car park A is full or closed\".", c: ["Health probes", "Health checks", "Health checks"], why: "Signs only change if someone checks and reports." },
    "r-turnstiles": { perm: true, related: ["r-bridge"], name: "Turnstiles", what: "More turnstiles open when the queue grows.", c: ["VM Scale Sets, AKS autoscaler, Container Apps scaling", "Auto Scaling, Karpenter", "Managed instance groups, GKE autoscaler, Cloud Run"], why: "Handle the rush, then shrink after." },
    "r-queue": { perm: true, related: ["r-bridge"], name: "Barriers and a \"wait here\" queue", what: "Fans wait calmly in a queue, not in a crush.", c: ["API Management limits, Service Bus queues", "API Gateway throttling, SQS", "Apigee quotas, Pub/Sub"], why: "Queues level the load so nothing is overwhelmed." },
    "r-bridge": { perm: true, related: ["r-queue", "r-turnstiles"], name: "Footbridge with a weight limit", what: "The bridge to the stadium. It can only hold so many people at once.", c: ["Your app's real capacity", "Your app's real capacity", "Your app's real capacity"], why: "Weight limits must be enforced at the entrance (rate limits, load shedding), not just written on a sign." },
    "r-jitter": { perm: true, related: ["r-bridge"], name: "Leaving stand by stand", what: "Fans leave a few at a time, not all at once.", c: ["Retry with backoff and jitter", "Retry with backoff and jitter", "Retry with backoff and jitter"], why: "Stops the thundering herd after an outage." }
  },

  /* The hierarchy, exactly as the CloudTyke dashboard shows it. */
  tree: [
    { id: "t-root", label: "Tenant Root Group", sub: "7c10d7e4-0000-4000-8000-000000000001", part: "land", depth: 0,
      c: ["Tenant Root Group", "Organization root", "Organization node"] },
    { id: "t-platform", label: "Platform", sub: "ct-platform", part: "shell", depth: 1, c: ["ct-platform", "Infrastructure / Security OUs", "platform folder"] },
    { id: "t-conn", label: "sub-connectivity", sub: "hub VNet, firewall, Bastion, DNS, ACR", part: "gates", depth: 2, c: ["sub-connectivity", "Network account", "Shared VPC host project"] },
    { id: "t-mgmt", label: "sub-management", sub: "Log Analytics", part: "control", depth: 2, c: ["sub-management", "Log Archive + Audit accounts", "Logging project"] },
    { id: "t-lz", label: "Landing Zones", sub: "ct-landingzones (safety code assigned here)", part: "pitch", depth: 1, c: ["ct-landingzones", "Workloads OU", "landing-zones folder"] },
    { id: "t-events", label: "sub-events", sub: "5b5c0000-0000-4000-8000-000000000004", part: "pitch", depth: 2, c: ["sub-events", "an event account", "an event project"] },
    { id: "t-sandbox", label: "Sandbox", sub: "ct-sandbox", part: "training", depth: 1, c: ["ct-sandbox", "Sandbox OU", "sandbox folder"] },
    { id: "t-subsandbox", label: "sub-sandbox", sub: "5b5c0000-0000-4000-8000-000000000003", part: "training", depth: 2, c: ["sub-sandbox", "a sandbox account", "a sandbox project"] }
  ],

  /* Event kits (section 4). */
  kits: {
    football: { name: "Football", event: "Saturday Derby", shape: "90 minutes. A huge rush at kick-off and half-time.",
      compute: "k8s", pool: "AKS autoscaling pool, min 3, max 30", storage: ["s-object"], db: false,
      needs: ["AKS with a big autoscaling pool (min 3, max 30) for tickets and the fan app", "Blob storage for highlight clips, kept a short time"] },
    cricket: { name: "Cricket", event: "Summer Test", shape: "Up to 5 days. Rain stops play.",
      compute: "k8s", pool: "small steady AKS pool", storage: ["s-object", "s-db"], db: true,
      needs: ["A small steady AKS pool", "PostgreSQL for scorecards and stats, kept for years", "Blob storage for ball-tracking replays: hot, then cool, then archive"] },
    concert: { name: "Concert", event: "Neon Nights", shape: "About 3 hours. A big rush at doors-open.",
      compute: "k8s", pool: "AKS autoscaling pool", storage: ["s-object"], db: false,
      needs: ["An AKS autoscaling pool", "Storage for stage media, deleted after 30 days", "No database"] }
  },

  /* Chapters: title, puzzle line, main ideas, the venue story card after it, and a 3-question check. */
  chapters: [
    { file: "01-build.html", n: 1, title: "Build the stadium", puzzle: "Place the 11 pieces in order.", ideas: "The whole landing zone. What is permanent and what changes per event.", story: "colosseum",
      quiz: [
        { q: "Which part is the only one that changes between events?", a: ["The gates", "The pitch", "The safety code"], ok: 1, why: "Gates, seats, security and rules stay. Only the pitch (the landing zone for one app) changes." },
        { q: "Where do the shared parts (gates, control room) live in CloudTyke?", a: ["ct-platform", "ct-sandbox", "ct-landingzones"], ok: 0, why: "Shared platform parts live under ct-platform, in sub-connectivity and sub-management." },
        { q: "Why build the gates before the pitch?", a: ["Gates are cheaper", "Every event needs the same safe way in, so it must exist first", "The pitch needs no network"], ok: 1, why: "The platform comes first. Then any event can land safely on it." }
      ] },
    { file: "02-gates.html", n: 2, title: "Gates, walkways and doors", puzzle: "Trace a fan. Fix broken walkways. Write door rules.", ideas: "Hub and spoke, peering is not transitive, routes to the firewall, NSG vs SG vs GCP firewall rules, stateful vs stateless.", story: "dropin",
      quiz: [
        { q: "Derby and Summer are both peered to the hub. Can Derby reach Summer directly?", a: ["Yes, peering passes through", "No, peering is not transitive. Traffic must be routed via the hub firewall", "Only on Saturdays"], ok: 1, why: "Peering joins two networks only. Use a route to the hub firewall to go further." },
        { q: "An NSG has Deny-all at priority 100 and Allow-443 at 200. What happens to HTTPS?", a: ["Allowed", "Denied, because the lowest number is checked first and wins", "Both rules apply"], ok: 1, why: "Rules are read from the lowest priority number up. The first match wins." },
        { q: "What does stateless mean for a door rule?", a: ["Replies need their own rule", "Replies are allowed automatically", "It has no rules"], ok: 0, why: "Stateless rules (like AWS network ACLs) do not remember the request, so the reply must be allowed too." }
      ] },
    { file: "03-safety-code.html", n: 3, title: "The safety code", puzzle: "Sort rules onto the right wall. Watch them flow down. Try to break them.", ideas: "Policy, inheritance down management groups, OUs and folders, deny errors.", story: "retractable",
      quiz: [
        { q: "A policy is assigned at ct-landingzones. Which subscriptions get it?", a: ["Only sub-connectivity", "Every subscription under ct-landingzones, like sub-events", "All subscriptions in the tenant"], ok: 1, why: "Policy flows down. Everything below the assignment inherits it." },
        { q: "You try to create a public IP in sub-events. What error do you get?", a: ["AuthorizationFailed", "RequestDisallowedByPolicy", "ResourceNotFound"], ok: 1, why: "The safety code (a deny policy) refuses the request before anything is built." },
        { q: "Why are the sandbox rules looser?", a: ["It is a safe place to practise, away from real events", "Nobody uses it", "Policy does not work there"], ok: 0, why: "The training ground lets people learn. Its own lighter rules still apply." }
      ] },
    { file: "04-safe.html", n: 4, title: "The club safe", puzzle: "Find the key taped to the door. Swap it for a wristband.", ideas: "Vault, secrets, keys, certificates, workload identity, purge protection.", story: "ice",
      quiz: [
        { q: "What is the best way for an app to read a database password?", a: ["Put it in appsettings.json", "From the vault, using a managed identity", "Email it to the team"], ok: 1, why: "A managed identity is a wristband. The app proves who it is and needs no password to open the vault." },
        { q: "What does purge protection stop?", a: ["Reading secrets", "Deleting a vault or key for good, even by mistake", "Creating new secrets"], ok: 1, why: "Deleted items wait in a recycle bin and cannot be purged early." },
        { q: "The Derby app tries to read the Summer locker. What should happen?", a: ["It works", "It is refused, each app opens only its own locker", "The vault is deleted"], ok: 1, why: "Least privilege: give each identity access to its own secrets only." }
      ] },
    { file: "05-control-room.html", n: 5, title: "The control room", puzzle: "Follow one fan on CCTV. Find the slow turnstile.", ideas: "Traces, metrics, logs, OpenTelemetry, alerts.", story: "ramoji",
      quiz: [
        { q: "One fan waited ten minutes. Which signal shows which step was slow?", a: ["A trace", "A metric", "A dashboard title"], ok: 0, why: "A trace follows one request and times each step." },
        { q: "What is OpenTelemetry?", a: ["A camera brand", "A standard way to send traces, metrics and logs to any backend", "A database"], ok: 1, why: "It is the standard fitting. Add it once and plug into any control room." },
        { q: "Why are logs kept in sub-management and not in the event?", a: ["To save space", "So the tapes stay after the event leaves, and an event cannot wipe them", "No reason"], ok: 1, why: "The control room is never rebuilt and keeps the tapes." }
      ] },
    { file: "06-changeover.html", n: 6, title: "The pitch and the changeover", puzzle: "Build tonight's kit. Change the event. Count what was rebuilt.", ideas: "Event kits, native app, Docker, Kubernetes, storage, stateless vs stateful.", story: "hengdian",
      quiz: [
        { q: "You change the Derby from football to concert. What should Terraform change?", a: ["Gates and seats", "Only that event's pitch", "The safety code"], ok: 1, why: "Changing one kit value swaps only the pitch. Gates, seats, the control room and the rules stay." },
        { q: "Where should years of cricket stats live?", a: ["Inside a pod", "In a database", "On the scoreboard screen"], ok: 1, why: "Pods come and go. Stateful data lives in storage that outlasts them." },
        { q: "Which is a flat-pack set piece, the same on every stage?", a: ["A Docker container", "A hand-built VM", "A firewall"], ok: 0, why: "A container image is built once and runs the same everywhere." }
      ] },
    { file: "07-team.html", n: 7, title: "The team", puzzle: "Pack the squad. Seat the crowd.", ideas: "Kubernetes as the manager, bin packing, requests and limits, spreading.", story: "colosseum2",
      quiz: [
        { q: "Why must every pod set resource requests?", a: ["For fun", "The scheduler can only pack well if each pod says what it needs", "Requests make pods faster"], ok: 1, why: "No requests means the scheduler packs blind." },
        { q: "You packed all ticket pods on one node. That node fails. What is the lesson?", a: ["Pack even tighter", "Spread important pods across nodes and zones", "Turn off autoscaling"], ok: 1, why: "Packing saves money. Spreading the important pods keeps you safe." },
        { q: "Single empty seats nobody can buy are like…", a: ["Fragmentation: spare room that no pod can use", "Autoscaling", "A PodDisruptionBudget"], ok: 0, why: "Spare CPU on one node and spare memory on another, but no node fits the next pod." }
      ] },
    { file: "08-rush.html", n: 8, title: "Match day rush", puzzle: "Open the overflow car park. Keep the bridge green.", ideas: "Autoscaling, failover, zones and regions, rate limits, jitter, the bridge stories.", story: "studio",
      quiz: [
        { q: "The main region fails. The second region is half the size. What happens?", a: ["All fine", "The crowd moves to a place that also breaks", "Traffic stops for ever"], ok: 1, why: "The overflow car park must be big enough. Reserve and test the capacity." },
        { q: "After an outage, every client retries at the same second. What helps?", a: ["Retry faster", "Backoff with jitter, queues and rate limits", "Turn off logs"], ok: 1, why: "Spread retries out so the system can recover." },
        { q: "Where should a weight limit be enforced?", a: ["On a sign", "At the entrance, before people step on", "After the bridge"], ok: 1, why: "Admission control at the door. Refuse or queue early (429 or 503)." }
      ] },
    { file: "09-your-event.html", n: 9, title: "Your own event", puzzle: "Design a new event from a short brief, then get it checked.", ideas: "Everything together.", story: null,
      quiz: [
        { q: "A new team needs a space. What is the fastest safe way?", a: ["Build a new stadium", "Book it at the box office: subscription vending", "Share another event's pitch"], ok: 1, why: "Vending gives a ready landing zone with seats, gates and rules already in place." },
        { q: "Which parts do you design for a new event?", a: ["Only the pitch kit", "Gates and firewall", "The tenant root"], ok: 0, why: "The platform is already there. You choose the pitch." },
        { q: "Your design has a public IP. What will the safety code do?", a: ["Allow it", "Deny it with RequestDisallowedByPolicy", "Delete the stadium"], ok: 1, why: "No public IPs in events. Traffic comes in through the gates." }
      ] }
  ],

  /* Real venue stories (cards). Sources are linked in the README. */
  stories: {
    colosseum: { title: "The Colosseum, Rome", icon: "colosseum", line: "Around AD 90 an underground level of lifts and trapdoors was added. It raised fighters, animals and whole sets into the arena.", lesson: "The hidden machinery is the platform: built once, it makes every show fast. But it also ended the flooded sea battles. Platform choices limit what you can host." },
    colosseum2: { title: "The Colosseum's sailors", icon: "awning", line: "A huge sun awning was worked by sailors from the navy.", lesson: "A specialist platform team runs the hard shared parts, so event crews do not have to." },
    dropin: { title: "Drop-in cricket pitches", icon: "dropin", line: "At grounds like the MCG and Adelaide Oval the cricket strip is grown in a tray outside, then craned in. New York's 2024 T20 World Cup pitches were grown in Florida.", lesson: "Build and test the event kit somewhere else, then drop it in. That is a landing zone template built by a pipeline." },
    retractable: { title: "Retractable pitches", icon: "retract", line: "At the GelreDome (1998), Veltins-Arena, Sapporo Dome and Tottenham the grass slides out (Tottenham's in three parts) to reveal a hard floor.", lesson: "Protect the precious workload and swap the layer on top. Fast, planned, repeatable changeovers." },
    ice: { title: "Indoor arenas", icon: "ice", line: "The ice stays. Crews cover it with insulated panels and build a basketball court or concert floor on top, often overnight.", lesson: "The base stays, the event layer goes on top. The overnight crew is the pipeline. Their checklist is the runbook." },
    ramoji: { title: "Ramoji Film City, Hyderabad", icon: "ramoji", line: "Guinness World Records calls it the largest film studio complex. Stages, power, props and editing are already there, so a producer can arrive with a script.", lesson: "The studio lot is the platform. Each sound stage is a landing zone. Each set is a workload." },
    hengdian: { title: "Hengdian World Studios, China", icon: "hengdian", line: "A huge outdoor lot with a life-size copy of the Forbidden City. Often around twenty productions shoot at once.", lesson: "Many productions share the lot, but each has its own stage and crew. That is isolation between landing zones." },
    studio: { title: "Same lot, a new set every week", icon: "ramoji", line: "Film crews build sets fast, use them, then change them for the next production. Some sets stay for others to reuse.", lesson: "Swap the workload, keep the platform. Reuse what is proven." }
  },
  storyStrip: ["colosseum", "dropin", "retractable", "ice", "ramoji", "hengdian"],

  /* Bridges (chapter 8). Real people died in some of these. Tell them with care. */
  bridges: [
    { name: "Morbi, India (2022)", what: "An old suspension bridge reopened after a renovation without a fitness certificate or a load test. A heavier deck had been added and the cables were corroded. Thousands of tickets were sold. About 300 people were on it when it fell.", lesson: "Limit how many get in at the door. Load-test every change before go-live. No release without sign-off." },
    { name: "Millennium Bridge, London (2000)", what: "It did not fall. On opening day walkers fell into step with a small sway, which made it worse. It closed and was fixed with dampers.", lesson: "Clients that act in sync (retry storms) shake the system. Add dampers: jitter, backoff and rate limits." },
    { name: "Silver Bridge, USA (1967)", what: "One small crack in one steel link brought the whole bridge down. Each link had only two bars and no backup.", lesson: "No single point of failure. Use many zones, replicas and paths, like a cable made of many wires." },
    { name: "Hyatt Regency walkways, USA (1981)", what: "A simple-looking change during building doubled the load on one connection. Nobody re-checked the maths.", lesson: "Review every change, even small ones. Use pull requests, plans and policy checks." },
    { name: "Tacoma Narrows, USA (1940)", what: "Strong enough for its weight, but wind made it twist more and more until it broke.", lesson: "Test real-world behaviour, not just size. Watch for feedback loops like autoscaler flapping and retry storms." },
    { name: "I-35W, Minneapolis (2007)", what: "Some steel plates were designed at half the right thickness. It stood for 40 years until weight was added over time, plus building materials on the day.", lesson: "Hidden weak spots wait for a busy day. Re-check capacity as things grow, and keep monitoring." }
  ],

  /* Final 12-question quiz (chapter 9). About ideas, not trivia. */
  finalQuiz: [
    { q: "In one sentence, what is a landing zone?", a: ["A single virtual machine", "A ready, governed space where a team can safely run an app, on shared platform parts", "A login page"], ok: 1, why: "It is the stadium (shared, built once) plus a ready pitch for each event." },
    { q: "Which parts should never be rebuilt for a new event?", a: ["Gates, seats, security and rules", "The app and its containers", "The event's own storage"], ok: 0, why: "Only the pitch changes. The platform stays." },
    { q: "Why have one way in?", a: ["It looks nice", "One place to check, log and block traffic", "It is faster"], ok: 1, why: "Side doors (public IPs in events) skip the bag check." },
    { q: "Why is peering not enough for spoke-to-spoke traffic?", a: ["Peering is not transitive", "Peering is too slow", "Peering costs nothing"], ok: 0, why: "Route via the hub firewall instead." },
    { q: "A rule assigned at a management group…", a: ["applies to that group only", "flows down to everything beneath it", "needs to be copied to each subscription"], ok: 1, why: "Inheritance is the point. Set it once, high enough." },
    { q: "Where should an app's database password live?", a: ["In the code", "In the vault, read with a managed identity", "In a shared spreadsheet"], ok: 1, why: "A wristband, not a key taped to the door." },
    { q: "One request was slow. What do you look at first?", a: ["A trace", "The bill", "The firewall rules"], ok: 0, why: "Traces time each step of one request." },
    { q: "Pods are replaced all the time. Where must important data live?", a: ["In the pod", "In storage that outlasts pods: a database, blob store or volume", "In the logs"], ok: 1, why: "Stateless compute, stateful storage." },
    { q: "Bin packing saves money. What is the risk if you pack too tight?", a: ["None", "One node or zone failure takes out many pods", "Pods get bigger"], ok: 1, why: "Spread the important pods with anti-affinity and topology spread." },
    { q: "Why must the second region be big enough?", a: ["Otherwise failover moves the crowd somewhere that also breaks", "It is a legal rule", "To look good"], ok: 0, why: "Reserve the capacity and test it with drills." },
    { q: "What stops a retry storm after an outage?", a: ["Retrying at once", "Backoff with jitter, queues and rate limits", "Bigger logs"], ok: 1, why: "Spread the clients out, like fans leaving stand by stand." },
    { q: "How does a new team get its own landing zone fast?", a: ["Subscription vending from a template, through a pipeline", "Ask for admin on the tenant root", "Copy another team's resources by hand"], ok: 0, why: "Book at the box office. The crew builds it the same way every time." }
  ],

  /* Chapter 7 tables (section 2 of the brief). */
  packTable: [
    ["Each player's role and how much running they need", "resource requests and limits (CPU, memory)", "The scheduler can only pack well if every pod says what it needs."],
    ["Fitness coach measures what each player really needs", "right-sizing (Vertical Pod Autoscaler recommendations)", "Too much wastes space. Too little causes slowdowns and evictions."],
    ["Fill one stand before opening the next", "bin packing: MostAllocated scoring, or Karpenter consolidation (AKS Node Auto Provisioning, EKS Karpenter, GKE optimize-utilization)", "Fewer nodes, lower cost."],
    ["Spread fans across all stands", "spreading: the default LeastAllocated style", "More headroom, but more nodes."],
    ["Don't put both centre-backs on one side", "pod anti-affinity, topology spread across zones", "One node or zone failure must not lose the whole defence."],
    ["Only the goalkeeper plays in goal", "taints and tolerations, node pools (GPU or memory pools)", "Special nodes only for pods that need them."],
    ["Keep the striker near the playmaker", "pod affinity", "Low latency between parts that talk a lot."],
    ["The star player always gets picked", "PriorityClass and preemption", "Key apps win when space is short."],
    ["\"At least 10 must stay on the pitch\"", "PodDisruptionBudget", "Safe upgrades and repacking without losing the match."],
    ["Half-time tactical reshuffle", "descheduler or consolidation moves pods to pack them again", "The pitch stays tidy as the game changes."],
    ["More stewards at the gates when queues grow", "Horizontal Pod Autoscaler, then the cluster autoscaler adds nodes", "Handle the kick-off rush, then shrink after."]
  ],
  seatTable: [
    ["The seat plan changes per event: a stage blocks seats (\"kill seats\"), the floor becomes standing room", "the same nodes, a different shape of workload per event"],
    ["Fill block by block, don't scatter fans over every stand", "bin packing onto fewer nodes"],
    ["Single empty seats nobody can buy (\"orphan seats\")", "fragmentation: spare CPU on one node, spare memory on another, but no node fits the next pod"],
    ["A family of five must sit together", "pod affinity and gang scheduling (place a group together or not at all)"],
    ["Wheelchair spaces only for those who need them", "taints and tolerations, special node pools"],
    ["Away fans in their own block with empty seats as a buffer", "namespaces, network policies and separate node pools for isolation"],
    ["Some seats held back for late VIPs and stewards", "reserved capacity for system pods (kube-reserved) and headroom for spikes"],
    ["Open the upper tier only when the lower tier is full", "the cluster autoscaler adds a node only when pods cannot fit"],
    ["Close an empty stand after the crowd leaves", "scale down and consolidate to cut cost"]
  ],
  /* Squads for "Pack the squad": [app, count, cpu, memGiB, important]. rush = extra pods for the big moment. */
  squads: {
    football: { pods: [["ticket-api", 4, 1, 2, true], ["fan-app", 4, 0.5, 1, false], ["clips", 2, 1, 3, false]], rush: [["ticket-api", 4, 1, 2, true], ["fan-app", 4, 0.5, 1, false]], rushLabel: "Kick-off rush", rushNote: "Kick-off! The Horizontal Pod Autoscaler adds 8 pods. If they cannot fit, the cluster autoscaler adds nodes." },
    cricket: { pods: [["scoreboard", 2, 0.5, 1, true], ["stats-api", 2, 1, 2, true], ["replay", 2, 1, 3, false]], rush: [["fan-app", 2, 0.5, 1, false]], rushLabel: "Lunch break", rushNote: "Small and steady: lunch only adds 2 pods. No need for a big pool." },
    concert: { pods: [["doors-app", 4, 0.5, 1, true], ["media", 2, 1, 3, false], ["merch", 2, 0.5, 1, false]], rush: [["doors-app", 6, 0.5, 1, true]], rushLabel: "Doors open", rushNote: "Doors open! 6 more door pods arrive at once." }
  },

  /* Chapter 1 puzzle: 11 pieces in build order, each with a riddle. */
  build: [
    { part: "land", riddle: "I am under everything. I have one owner, and nothing sits above me.", tree: ["t-root"] },
    { part: "shell", riddle: "I am the walls and roof. Shared parts live inside me, built once.", tree: ["t-platform"] },
    { part: "gates", riddle: "Every fan must pass me. There is one way in, and I am it.", tree: ["t-conn"] },
    { part: "stewards", riddle: "I check wristbands. Your band says where you may go.", tree: [] },
    { part: "rules", riddle: "I am written on a board by the gate. I am the same for every event.", tree: ["t-lz"] },
    { part: "control", riddle: "I watch every gate and stand on screens, and I keep the tapes.", tree: ["t-mgmt"] },
    { part: "safe", riddle: "I hold the trophies, the wallets and the private files. Only the right person may open each door.", tree: [] },
    { part: "kitroom", riddle: "I hand out tested gear to every event from one trusted shelf.", tree: [] },
    { part: "stands", riddle: "Same rows, same exits, same signs for every event. Nobody hand-builds me.", tree: [] },
    { part: "pitch", riddle: "I am the only part that changes. Tonight football, next week cricket.", tree: ["t-lz", "t-events"] },
    { part: "training", riddle: "I sit next door. Practise here. Mistakes never touch a real event.", tree: ["t-sandbox", "t-subsandbox"] }
  ]
};
