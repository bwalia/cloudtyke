# ================================================================ the organisation
#
# Contoso Pay is a fictional payment service provider (PSP). Merchants such as
# the fictional Northwind Airways send card payments to the Contoso Pay
# gateway, which scores them for fraud, authorises them with acquirers and
# settles funds daily. Everything below is invented.

data "azurerm_client_config" "current" {}
data "azurerm_subscription" "prod" {}

data "azuread_domains" "default" {
  only_default = true
}

resource "random_string" "suffix" {
  length  = 4
  upper   = false
  special = false
}

locals {
  domain = data.azuread_domains.default.domains[0].domain_name
  rc     = "uks" # region code
  sfx    = random_string.suffix.result

  teams = {
    platform   = { name = "Platform Engineering", cost_centre = "CC-1000", repo = "platform-infra" }
    payments   = { name = "Payments Core", cost_centre = "CC-2100", repo = "payment-gateway" }
    risk       = { name = "Risk & Fraud", cost_centre = "CC-2200", repo = "risk-engine" }
    merchant   = { name = "Merchant Experience", cost_centre = "CC-3100", repo = "merchant-portal" }
    settlement = { name = "Data & Settlement", cost_centre = "CC-4100", repo = "settlement-batch" }
    secops     = { name = "Security Operations", cost_centre = "CC-5000", repo = "secops-tooling" }
    sre        = { name = "Site Reliability", cost_centre = "CC-1100", repo = "sre-runbooks" }
  }

  # 30 invented people. Group membership drives every permission.
  people = {
    "hannah.okoro"      = { name = "Hannah Okoro", title = "Head of Platform", team = "platform", groups = ["platform-admins", "platform-leads"] }
    "marco.bianchi"     = { name = "Marco Bianchi", title = "Senior Platform Engineer", team = "platform", groups = ["platform-admins"] }
    "aisha.rahman"      = { name = "Aisha Rahman", title = "Platform Engineer", team = "platform", groups = ["platform-engineers"] }
    "jonas.weber"       = { name = "Jonas Weber", title = "Platform Engineer", team = "platform", groups = ["platform-engineers"] }
    "chloe.martin"      = { name = "Chloe Martin", title = "Cloud Network Engineer", team = "platform", groups = ["platform-engineers", "network-admins"] }
    "samuel.adeyemi"    = { name = "Samuel Adeyemi", title = "SRE Lead", team = "sre", groups = ["sre-oncall", "sre-leads"] }
    "nina.petrova"      = { name = "Nina Petrova", title = "Site Reliability Engineer", team = "sre", groups = ["sre-oncall"] }
    "oliver.hughes"     = { name = "Oliver Hughes", title = "Site Reliability Engineer", team = "sre", groups = ["sre-oncall"] }
    "priya.nair"        = { name = "Priya Nair", title = "Engineering Manager, Payments", team = "payments", groups = ["payments-leads", "pci-cde-access"] }
    "tom.fischer"       = { name = "Tom Fischer", title = "Staff Engineer, Payments", team = "payments", groups = ["payments-engineers", "payments-dba", "pci-cde-access"] }
    "lucia.garcia"      = { name = "Lucia Garcia", title = "Senior Backend Engineer", team = "payments", groups = ["payments-engineers", "pci-cde-access"] }
    "kenji.tanaka"      = { name = "Kenji Tanaka", title = "Backend Engineer", team = "payments", groups = ["payments-engineers"] }
    "fatima.zahra"      = { name = "Fatima Zahra", title = "Backend Engineer", team = "payments", groups = ["payments-engineers"] }
    "david.cohen"       = { name = "David Cohen", title = "Database Reliability Engineer", team = "payments", groups = ["payments-dba", "pci-cde-access"] }
    "elena.popescu"     = { name = "Elena Popescu", title = "Head of Risk", team = "risk", groups = ["risk-leads"] }
    "arjun.mehta"       = { name = "Arjun Mehta", title = "ML Engineer, Fraud", team = "risk", groups = ["risk-engineers"] }
    "grace.kim"         = { name = "Grace Kim", title = "Fraud Analyst", team = "risk", groups = ["risk-analysts"] }
    "ibrahim.diallo"    = { name = "Ibrahim Diallo", title = "Fraud Analyst", team = "risk", groups = ["risk-analysts"] }
    "sophie.dubois"     = { name = "Sophie Dubois", title = "Engineering Manager, Merchants", team = "merchant", groups = ["merchant-leads"] }
    "ryan.oconnor"      = { name = "Ryan O'Connor", title = "Senior Frontend Engineer", team = "merchant", groups = ["merchant-engineers"] }
    "mei.lin"           = { name = "Mei Lin", title = "Full-stack Engineer", team = "merchant", groups = ["merchant-engineers"] }
    "carlos.mendes"     = { name = "Carlos Mendes", title = "Merchant Integration Engineer", team = "merchant", groups = ["merchant-engineers"] }
    "anna.kowalska"     = { name = "Anna Kowalska", title = "Product Manager, Merchant Portal", team = "merchant", groups = ["merchant-readers"] }
    "george.white"      = { name = "George White", title = "Data Engineering Lead", team = "settlement", groups = ["settlement-leads", "pci-cde-access"] }
    "zainab.hussain"    = { name = "Zainab Hussain", title = "Data Engineer", team = "settlement", groups = ["settlement-engineers"] }
    "lars.andersen"     = { name = "Lars Andersen", title = "Reconciliation Analyst", team = "settlement", groups = ["settlement-readers"] }
    "rachel.green"      = { name = "Rachel Green", title = "CISO", team = "secops", groups = ["secops-leads", "security-readers"] }
    "victor.morales"    = { name = "Victor Morales", title = "Security Engineer", team = "secops", groups = ["security-readers", "policy-admins"] }
    "yuki.sato"         = { name = "Yuki Sato", title = "Detection Engineer", team = "secops", groups = ["security-readers"] }
    "breakglass.admin1" = { name = "Break Glass Admin 1", title = "Emergency access account", team = "secops", groups = ["breakglass"] }
  }

  groups = {
    "platform-admins"      = "Owner of the shared platform (hub, AKS, logging)."
    "platform-leads"       = "Approvers for platform changes."
    "platform-engineers"   = "Operate the platform; contributor on shared RGs."
    "network-admins"       = "Hub network, peerings, DNS."
    "sre-oncall"           = "Read everywhere, operate AKS during incidents."
    "sre-leads"            = "Incident commanders."
    "payments-leads"       = "Owners of the payments workload."
    "payments-engineers"   = "Deploy and operate the payment gateway."
    "payments-dba"         = "Entra administrators of the payments PostgreSQL server."
    "risk-leads"           = "Owners of the risk engine."
    "risk-engineers"       = "Build fraud models and the risk engine."
    "risk-analysts"        = "Review fraud signals (read-only)."
    "merchant-leads"       = "Owners of the merchant portal."
    "merchant-engineers"   = "Build the merchant portal and storefront demos."
    "merchant-readers"     = "Product and support: read-only."
    "settlement-leads"     = "Owners of settlement and reconciliation."
    "settlement-engineers" = "Run settlement batches."
    "settlement-readers"   = "Read settlement reports."
    "secops-leads"         = "Security leadership."
    "security-readers"     = "Read every subscription and the security logs."
    "policy-admins"        = "Manage Azure Policy (guardrails)."
    "pci-cde-access"       = "Approved for access to the cardholder data environment (quarterly review)."
    "breakglass"           = "Emergency access only; every use is audited."
  }

  tags = {
    environment         = var.environment
    data-classification = "pci-cde"
    managed-by          = "terraform"
  }
  team_tags = { for k, t in local.teams : k => merge(local.tags, { team = t.name, cost-centre = t.cost_centre }) }
}
