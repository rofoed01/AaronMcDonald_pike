resource "google_privileged_access_manager_entitlement" "entitlement" {
  entitlement_id       = "example-entitlement"
  location             = "global"
  max_request_duration = "43200s"
  parent               = "projects/pike"
  requester_justification_config {
    unstructured {}
  }
  eligible_users {
    principals = [
      "group:test@google.com"
    ]
  }
  privileged_access {
    gcp_iam_access {
      role_bindings {
        role                 = "roles/storage.admin"
        condition_expression = "request.time < timestamp(\"2024-04-23T18:30:00.000Z\")"
      }
      resource      = "//cloudresourcemanager.googleapis.com/projects/my-project-name"
      resource_type = "cloudresourcemanager.googleapis.com/Project"
    }
  }
  additional_notification_targets {
    admin_email_recipients = [
      "user@example.com",
    ]
    requester_email_recipients = [
      "user@example.com"
    ]
  }
  approval_workflow {
    manual_approvals {
      require_approver_justification = true
      steps {
        approvals_needed = 1
        approver_email_recipients = [
          "james.woolfenden@gmail.com"
        ]
        approvers {
          principals = [
            "group:james.woolfenden@gmail.com"
          ]
        }
      }
    }
  }
}
