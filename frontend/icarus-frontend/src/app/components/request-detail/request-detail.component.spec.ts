import '@angular/compiler';
import { describe, it, expect, beforeEach } from 'vitest';
import { RequestDetailComponent } from './request-detail.component';
import { WorkflowService, AuditLog, AccessCart } from '../../services/workflow.service';
import { of, throwError } from 'rxjs';

describe('RequestDetailComponent', () => {
  let mockWorkflowService: { getAuditTrail: any; getCart: any; bumpCart: any; withdrawCart: any };
  let mockRoute: any;
  let mockRouter: any;
  let mockLocation: any;

  const mockCart: AccessCart = {
    id: 'cart-123',
    requester_id: 'alice@example.com',
    status: 'IN_PROGRESS',
    justification: 'Need access for production debugging',
    created_at: '2026-09-09T08:00:00Z',
    submitted_at: '2026-09-09T09:00:00Z',
    items: [
      {
        id: 'item-1',
        cart_id: 'cart-123',
        role_id: 'auth-ms:Admin',
        role_name: 'Admin',
        status: 'PENDING',
        execution_id: 'exec-456',
        created_at: '2026-09-09T08:00:00Z'
      }
    ],
    pending_steps: [
      {
        step_id: 'step-001',
        role_name: 'Admin',
        cart_id: 'cart-123',
        assigned_to_user_id: 'bob@example.com'
      }
    ]
  };

  const mockEvents: AuditLog[] = [
    {
      id: 'event-1',
      entity_type: 'CART',
      entity_id: 'cart-123',
      actor_user_id: 'alice@example.com',
      action: 'SUBMITTED',
      after_state: '{"status":"SUBMITTED"}',
      correlation_id: 'corr-001',
      occurred_at: '2026-09-09T10:00:00Z'
    },
    {
      id: 'event-2',
      entity_type: 'WORKFLOW_STEP',
      entity_id: 'step-456',
      actor_user_id: 'bob@example.com',
      action: 'APPROVED',
      after_state: '{"decision":"APPROVE","comment":"LGTM"}',
      correlation_id: 'corr-002',
      occurred_at: '2026-09-09T11:00:00Z'
    }
  ];

  beforeEach(() => {
    mockWorkflowService = {
      getAuditTrail: (id: string) => of({ events: mockEvents }),
      getCart: (id: string) => of(mockCart),
      bumpCart: (id: string) => of({ message: 'Approvers notified' }),
      withdrawCart: (id: string) => of({ message: 'Request withdrawn' })
    };
    mockRoute = {
      snapshot: {
        paramMap: {
          get: (key: string) => (key === 'instanceId' ? 'cart-123' : null)
        },
        queryParamMap: {
          get: (key: string) => null
        }
      }
    };
    mockRouter = {
      navigate: () => {},
      navigateByUrl: () => {}
    };
    mockLocation = {
      back: () => {}
    };
  });

  it('should map actions to semantic statuses correctly', () => {
    const instance = Object.create(RequestDetailComponent.prototype) as RequestDetailComponent;

    expect(instance.mapActionToStatus('APPROVED')).toBe('completed');
    expect(instance.mapActionToStatus('COMPLETED')).toBe('completed');
    expect(instance.mapActionToStatus('AUTO_APPROVED')).toBe('completed');
    expect(instance.mapActionToStatus('REJECTED')).toBe('error');
    expect(instance.mapActionToStatus('DELEGATED')).toBe('warning');
    expect(instance.mapActionToStatus('STARTED')).toBe('in-progress');
    expect(instance.mapActionToStatus('SUBMITTED')).toBe('primary');
    expect(instance.mapActionToStatus('UNKNOWN')).toBe('neutral');
  });

  it('should map cart status to status tag variants correctly', () => {
    const instance = Object.create(RequestDetailComponent.prototype) as RequestDetailComponent;

    expect(instance.mapStatus('APPROVED')).toBe('success');
    expect(instance.mapStatus('COMPLETED')).toBe('success');
    expect(instance.mapStatus('REJECTED')).toBe('danger');
    expect(instance.mapStatus('CANCELLED')).toBe('danger');
    expect(instance.mapStatus('IN_PROGRESS')).toBe('warning');
    expect(instance.mapStatus('SUBMITTED')).toBe('warning');
    expect(instance.mapStatus('DRAFT')).toBe('info');
    expect(instance.mapStatus('ARCHIVED')).toBe('info');
    expect(instance.mapStatus(undefined)).toBe('neutral');
  });

  it('should navigate back correctly in goBack() based on query parameters or history', () => {
    let navigatedTo: any[] = [];
    let backCalled = false;

    const testInstance = Object.create(RequestDetailComponent.prototype) as any;
    testInstance.location = { back: () => { backCalled = true; } };
    testInstance.router = { navigate: (path: any[]) => { navigatedTo = path; }, navigateByUrl: (url: string) => { navigatedTo = [url]; } };

    // Test with returnUrl
    testInstance.route = {
      snapshot: {
        queryParamMap: {
          get: (k: string) => (k === 'returnUrl' ? '/dashboard/custom' : null)
        }
      }
    };
    testInstance.goBack();
    expect(navigatedTo).toEqual(['/dashboard/custom']);

    // Test with from = management
    testInstance.route = {
      snapshot: {
        queryParamMap: {
          get: (k: string) => (k === 'from' ? 'management' : null)
        }
      }
    };
    testInstance.goBack();
    expect(navigatedTo).toEqual(['/dashboard/request-management']);

    // Test with from = my-requests
    testInstance.route = {
      snapshot: {
        queryParamMap: {
          get: (k: string) => (k === 'from' ? 'my-requests' : null)
        }
      }
    };
    testInstance.goBack();
    expect(navigatedTo).toEqual(['/dashboard/my-requests']);
  });
});
