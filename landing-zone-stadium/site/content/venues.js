/* Venue variations. The same lessons play as football, cricket, a concert or a film studio.
   Chapters read names and examples from here. Anything not set falls back to content.js. */
window.LZS.venues = {
  order: ["football", "cricket", "concert", "studio"],
  football: {
    label: "Football", event: "Saturday Derby", kit: "football", person: "fan", people: "fans", place: "stadium",
    blurb: "A 90-minute match with a huge rush at kick-off.",
    names: { pitch: "The pitch (football tonight)" },
    teamTitle: "The manager and the team",
    team: {
      "k-control": ["The manager and coaching staff", "They steer the team and keep it matching the plan."],
      "k-manifest": ["The team sheet and formation", "Written before kick-off: who plays and where."],
      "k-scheduler": ["The manager picks positions", "Decides who plays where."],
      "k-pod": ["Players", "They do the work on the pitch."],
      "k-node": ["Positions on the pitch", "The places where players stand."],
      "k-deploy": ["\"We always want 11 on the pitch\"", "The number of players to keep."],
      "k-heal": ["Injury? A sub comes on", "A failed player is replaced."],
      "k-bench": ["Players on the bench", "Ready to come on when needed."],
      "k-kubelet": ["Coach on the touchline", "Reports back what is really happening."],
      "k-referee": ["The referee", "Stops illegal play."],
      "k-registry": ["The kit man", "Hands out the right kit every match."],
      "k-autoscale": ["More stewards when queues grow", "More help when it gets busy."]
    }
  },
  cricket: {
    label: "Cricket", event: "Summer Test", kit: "cricket", person: "fan", people: "fans", place: "ground",
    blurb: "Up to five days. Small and steady. Rain stops play.",
    names: { pitch: "The oval and the strip (cricket tonight)" },
    teamTitle: "The captain setting the field",
    team: {
      "k-control": ["The captain and the coach", "They steer the side and keep it matching the plan."],
      "k-manifest": ["The field plan", "Written before the over: who stands where."],
      "k-scheduler": ["The captain places fielders", "Placing fielders is like scheduling pods."],
      "k-pod": ["Fielders", "They do the work in the field."],
      "k-node": ["Fielding positions", "Slip, gully, cover, long-on."],
      "k-deploy": ["\"Always 11 in the field\"", "The number of fielders to keep."],
      "k-heal": ["A substitute fielder", "Someone hurt? A sub takes their place."],
      "k-bench": ["The twelfth player", "Waiting, ready to help."],
      "k-kubelet": ["The fielding coach on the boundary", "Reports back what is really happening."],
      "k-referee": ["The umpires", "Stop illegal play."],
      "k-registry": ["The kit bag in the pavilion", "The right gear, every session."],
      "k-autoscale": ["More drinks runners on a hot day", "More help when it gets busy."]
    }
  },
  concert: {
    label: "Concert", event: "Neon Nights", kit: "concert", person: "fan", people: "fans", place: "arena",
    blurb: "About three hours. A big rush at doors-open.",
    names: { pitch: "The stage and standing area (concert tonight)" },
    teamTitle: "The stage manager calling the cues",
    team: {
      "k-control": ["The stage manager", "Calls the cues and keeps the show matching the plan."],
      "k-manifest": ["The running order and cue sheet", "Written before doors open."],
      "k-scheduler": ["The stage manager places the crew", "Who works which part of the stage."],
      "k-pod": ["Crew and performers", "They do the work on the night."],
      "k-node": ["Stage positions", "Lighting rig, sound desk, wings."],
      "k-deploy": ["\"Always 4 on the lighting rig\"", "The number of crew to keep."],
      "k-heal": ["An understudy steps in", "Someone is ill? The show goes on."],
      "k-bench": ["Crew on standby", "Ready when a cue needs them."],
      "k-kubelet": ["Section chiefs on headsets", "Report back what is really happening."],
      "k-referee": ["The safety officer", "Stops anything unsafe."],
      "k-registry": ["The backline truck", "Instruments and gear, checked and ready."],
      "k-autoscale": ["More bar staff when the queue grows", "More help when it gets busy."]
    }
  },
  studio: {
    label: "Film studio", event: "Fabrikam Pictures shoot", kit: "concert", person: "visitor", people: "visitors", place: "studio lot",
    blurb: "A studio lot. Each sound stage is a landing zone. Each set is a workload.",
    lot: true,
    names: {
      land: "The studio lot's land", shell: "The lot wall", gates: "Studio gates", firewall: "Security check at the gate",
      staffdoor: "Crew-only gate", stands: "Standard stage fit-out (power, lights, fire exits)",
      pitch: "Sound stages (one per production)", control: "Security office and CCTV", safe: "The production safe",
      kitroom: "Prop warehouse", rules: "Lot safety code", stewards: "Crew passes",
      boxoffice: "Booking office: book a stage", crew: "Set-building crew", training: "Practice backlot"
    },
    teamTitle: "The floor manager and the crews",
    team: {
      "k-control": ["The floor manager", "Keeps the shoot matching the plan."],
      "k-manifest": ["The call sheet", "Written the night before: who works where."],
      "k-scheduler": ["The floor manager places crews", "Which crew goes to which stage."],
      "k-pod": ["Crew members", "Grips, sparks, camera operators."],
      "k-node": ["Sound stages", "The places where crews work."],
      "k-deploy": ["\"Always 6 in the lighting crew\"", "The number of crew to keep."],
      "k-heal": ["A replacement grip", "Someone is off? Another steps in."],
      "k-bench": ["Crew on call", "Ready when needed."],
      "k-kubelet": ["Heads of department", "Report back what is really happening."],
      "k-referee": ["The health and safety officer", "Stops anything unsafe."],
      "k-registry": ["The prop warehouse", "Tested pieces, the same on every stage."],
      "k-autoscale": ["More extras for a crowd scene", "More help when the scene needs it."]
    }
  }
};
