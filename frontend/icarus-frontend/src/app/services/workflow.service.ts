import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, fromEvent, merge, EMPTY } from 'rxjs';
import { map, switchMap, retry, delay } from 'rxjs/operators';
import { AuthService } from './auth.service';

export interface AccessCart {
  id: string;
  requester_id: string;
  status: string;
  justification: string;
  items: CartItem[];
  submitted_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface CartItem {
  id: string;
  cart_id: string;
  role_id: string;
  role_name: string;
  status: string;
  workflow_instance_id?: string;
  created_at: string;
}

export interface InboxItem {
  step_id: string;
  cart_item_id: string;
  cart_id: string;
  requester_id: string;
  role_requested: string;
  justification: string;
  stage_name: string;
  assignment_type: string;
  assigned_at: string;
}

export interface Workflow {
  id: string;
  name: string;
  description?: string;
  definition_key: string;
  version: number;
  is_current: boolean;
  status: string;
  supersedes_id?: string;
  nodes: WorkflowNode[];
  created_by: string;
  created_at: string;
}


export interface WorkflowNode {
  id: string;
  name: string;
  type: 'APPROVAL' | 'OPERATION';
  depends_on: string[];
  max_retries?: number;
  retry_interval?: number;
  retry_interval_seconds?: number;
  config?: Record<string, any>;
}

export interface WorkflowCandidate {
  type: 'USER' | 'ROLE';
  value: string;
}

export interface UpsertWorkflowRequest {
  name: string;
  description?: string;
  nodes: WorkflowNode[];
}

export interface WorkflowWithRoleMapping extends Workflow {
  mapped_role_id?: string;
}



export interface AuditLog {
  id: string;
  entity_type: string;
  entity_id: string;
  actor_user_id: string;
  action: string;
  before_state?: string;
  after_state?: string;
  correlation_id?: string;
  occurred_at: string;
}

export interface SseEvent {
  type: string;
  payload: Record<string, unknown>;
}

@Injectable({ providedIn: 'root' })
export class WorkflowService {
  private readonly http = inject(HttpClient);
  private readonly authService = inject(AuthService);

  private readonly baseUrl = 'http://localhost:8082/api/v1';

  // ── Cart ───────────────────────────────────────────────────────────────────

  createCart(justification: string): Observable<AccessCart> {
    return this.http.post<AccessCart>(`${this.baseUrl}/access/carts`, { justification });
  }

  listCarts(): Observable<AccessCart[]> {
    return this.http.get<AccessCart[]>(`${this.baseUrl}/access/carts`);
  }

  getCart(cartId: string): Observable<AccessCart> {
    return this.http.get<AccessCart>(`${this.baseUrl}/access/carts/${cartId}`);
  }

  addItemToCart(cartId: string, roleId: string, roleName: string): Observable<CartItem> {
    return this.http.post<CartItem>(`${this.baseUrl}/access/carts/${cartId}/items`, {
      role_id: roleId,
      role_name: roleName,
    });
  }

  removeItemFromCart(cartId: string, itemId: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/access/carts/${cartId}/items/${itemId}`);
  }

  submitCart(cartId: string): Observable<{ cart_id: string; status: string; message: string }> {
    return this.http.post<{ cart_id: string; status: string; message: string }>(
      `${this.baseUrl}/access/carts/${cartId}/submit`,
      {}
    );
  }

  // ── Inbox ──────────────────────────────────────────────────────────────────

  getInbox(): Observable<{ items: InboxItem[]; total: number }> {
    return this.http.get<{ items: InboxItem[]; total: number }>(`${this.baseUrl}/workflow/inbox`);
  }

  approveStep(stepId: string, comment: string): Observable<unknown> {
    return this.http.post(`${this.baseUrl}/workflow/steps/${stepId}/approve`, { comment });
  }

  rejectStep(stepId: string, comment: string): Observable<unknown> {
    return this.http.post(`${this.baseUrl}/workflow/steps/${stepId}/reject`, { comment });
  }

  delegateStep(stepId: string, delegateToUserId: string, comment: string): Observable<unknown> {
    return this.http.post(`${this.baseUrl}/workflow/steps/${stepId}/delegate`, {
      comment,
      delegate_to_user_id: delegateToUserId,
    });
  }

  // ── Audit Trail ────────────────────────────────────────────────────────────

  getAuditTrail(instanceId: string): Observable<{ instance_id: string; events: AuditLog[] }> {
    return this.http.get<{ instance_id: string; events: AuditLog[] }>(
      `${this.baseUrl}/workflow/instances/${instanceId}/audit-trail`
    );
  }

  // ── Workflow Definitions ───────────────────────────────────────────────────

  listWorkflowDefinitions(): Observable<Workflow[]> {
    return this.http.get<Workflow[]>(`${this.baseUrl}/workflow/definitions`);
  }

  getWorkflowDefinition(roleId: string): Observable<Workflow | null> {
    return this.http.get<Workflow | null>(
      `${this.baseUrl}/workflow/definitions/roles/${roleId}`
    );
  }

  getWorkflowDefinitionById(id: string): Observable<Workflow> {
    return this.http.get<Workflow>(`${this.baseUrl}/workflow/definitions/${id}`);
  }

  getDefinitionHistory(definitionKey: string): Observable<Workflow[]> {
    return this.http.get<Workflow[]>(
      `${this.baseUrl}/workflow/definitions/history/${definitionKey}`
    );
  }

  upsertWorkflowDefinition(roleId: string, req: UpsertWorkflowRequest): Observable<Workflow> {
    return this.http.put<Workflow>(
      `${this.baseUrl}/workflow/definitions/roles/${roleId}`,
      req
    );
  }

  /** Creates a standalone workflow template not yet bound to any role. */
  createWorkflowTemplate(req: UpsertWorkflowRequest): Observable<Workflow> {
    return this.http.post<Workflow>(`${this.baseUrl}/workflow/definitions`, req);
  }

  /** Lists all current workflow templates enriched with which role (if any) maps to each. */
  listWorkflowTemplates(): Observable<WorkflowWithRoleMapping[]> {
    return this.http.get<WorkflowWithRoleMapping[]>(`${this.baseUrl}/workflow/definitions/templates`);
  }

  /** Maps an existing workflow template to a role. */
  mapWorkflowToRole(roleId: string, workflowId: string): Observable<{ role_id: string; workflow_id: string; status: string }> {
    return this.http.put<{ role_id: string; workflow_id: string; status: string }>(
      `${this.baseUrl}/workflow/definitions/roles/${roleId}/map`,
      { workflow_id: workflowId }
    );
  }


  // ── SSE ────────────────────────────────────────────────────────────────────

  /**
   * Opens an SSE connection to the workflow service.
   * Returns an Observable<SseEvent> that emits on every server-sent event.
   * Automatically reconnects with exponential back-off on error.
   */
  connectSSE(): Observable<SseEvent> {
    const token = this.authService.accessToken();
    if (!token) {
      return EMPTY;
    }

    // NOTE: EventSource does not support custom headers. We pass the token as a
    // query param here for compatibility. The server should accept this fallback.
    const url = `${this.baseUrl}/workflow/events?token=${encodeURIComponent(token)}`;

    return new Observable<SseEvent>(subscriber => {
      const es = new EventSource(url);

      const handleEvent = (eventType: string) => (event: Event) => {
        try {
          const msgEvent = event as MessageEvent;
          subscriber.next({
            type: eventType,
            payload: JSON.parse(msgEvent.data),
          });
        } catch {
          // Ignore malformed events
        }
      };

      es.addEventListener('inbox.new', handleEvent('inbox.new'));
      es.addEventListener('cart.updated', handleEvent('cart.updated'));
      es.addEventListener('step.actioned', handleEvent('step.actioned'));

      es.onerror = () => {
        // Let the retry() operator handle reconnection
        subscriber.error(new Error('SSE connection lost'));
      };

      return () => es.close();
    }).pipe(
      retry({ delay: 5000 }) // Reconnect after 5 seconds on error
    );
  }
}
