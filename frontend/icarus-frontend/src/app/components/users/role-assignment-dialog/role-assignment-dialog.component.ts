import { Component, signal, computed, inject, ChangeDetectorRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DIALOG_DATA, DialogRef } from '@angular/cdk/dialog';
import { DragDropModule, CdkDragDrop, moveItemInArray } from '@angular/cdk/drag-drop';

interface Role {
  name: string;
  description: string;
}

interface DialogData {
  username: string;
  email: string;
  assignedRoles: string[];
  availableRoles: Role[];
}

@Component({
  selector: 'app-role-assignment-dialog',
  imports: [FormsModule, DragDropModule],
  templateUrl: './role-assignment-dialog.component.html',
  styleUrl: './role-assignment-dialog.component.scss'
})
export class RoleAssignmentDialogComponent {
  private readonly dialogRef = inject<DialogRef<string[]>>(DialogRef);
  private readonly cdr = inject(ChangeDetectorRef);
  protected readonly data = inject<DialogData>(DIALOG_DATA);

  // Signals-based approach: use signals for both lists so computed derives cleanly
  protected readonly availableListSig = signal<Role[]>([]);
  protected readonly assignedListSig = signal<string[]>([]);

  // Search query signals
  protected readonly roleSearchQuery = signal('');
  protected readonly assignedRoleSearchQuery = signal('');

  // Computed: filtered available (excludes assigned, matches query)
  protected readonly filteredAvailable = computed(() => {
    const q = this.roleSearchQuery().toLowerCase().trim();
    const list = this.availableListSig();
    if (!q) return list;
    return list.filter(r =>
      r.name.toLowerCase().includes(q) || r.description.toLowerCase().includes(q)
    );
  });

  // Computed: filtered assigned (matches query)
  protected readonly filteredAssigned = computed(() => {
    const q = this.assignedRoleSearchQuery().toLowerCase().trim();
    const list = this.assignedListSig();
    if (!q) return list;
    return list.filter(name => name.toLowerCase().includes(q));
  });

  // Computed: count
  protected readonly assignedCount = computed(() => this.assignedListSig().length);

  constructor() {
    const assignedSet = new Set(this.data.assignedRoles);
    this.assignedListSig.set([...this.data.assignedRoles]);
    this.availableListSig.set(this.data.availableRoles.filter(r => !assignedSet.has(r.name)));
  }

  // CDK DnD needs mutable arrays directly. We expose getters that return the signal value,
  // then after mutation we re-set the signal so change detection is correctly triggered.
  get availableList(): Role[] { return this.availableListSig(); }
  get assignedList(): string[] { return this.assignedListSig(); }

  // --- Drag & Drop Handler ---
  onDrop(event: CdkDragDrop<any[]>): void {
    if (event.previousContainer === event.container) {
      // Reorder within same list
      const arr = [...event.container.data];
      moveItemInArray(arr, event.previousIndex, event.currentIndex);
      if (event.container.id === 'assigned-list') {
        this.assignedListSig.set(arr as string[]);
      } else {
        this.availableListSig.set(arr as Role[]);
      }
    } else {
      const fromAvailable = event.previousContainer.id === 'available-list';
      if (fromAvailable) {
        const avail = [...this.availableListSig()];
        const role = avail[event.previousIndex];
        avail.splice(event.previousIndex, 1);
        const assigned = [...this.assignedListSig()];
        assigned.splice(event.currentIndex, 0, role.name);
        this.availableListSig.set(avail);
        this.assignedListSig.set(assigned);
      } else {
        const assigned = [...this.assignedListSig()];
        const roleName = assigned[event.previousIndex];
        assigned.splice(event.previousIndex, 1);
        const avail = [...this.availableListSig()];
        const roleObj = this.data.availableRoles.find(r => r.name === roleName);
        if (roleObj) avail.splice(event.currentIndex, 0, roleObj);
        this.assignedListSig.set(assigned);
        this.availableListSig.set(avail);
      }
    }
  }

  // --- Click-based add/remove ---
  addRole(role: Role): void {
    this.availableListSig.update(list => list.filter(r => r.name !== role.name));
    this.assignedListSig.update(list => {
      if (list.includes(role.name)) return list;
      return [...list, role.name];
    });
  }

  removeRole(roleName: string): void {
    this.assignedListSig.update(list => list.filter(r => r !== roleName));
    this.availableListSig.update(list => {
      if (list.find(r => r.name === roleName)) return list;
      const roleObj = this.data.availableRoles.find(r => r.name === roleName);
      return roleObj ? [...list, roleObj] : list;
    });
  }

  clearAllRoles(): void {
    const allRoleObjs = this.assignedListSig()
      .map(name => this.data.availableRoles.find(r => r.name === name))
      .filter((r): r is Role => !!r);
    this.assignedListSig.set([]);
    this.availableListSig.update(list => {
      const existing = new Set(list.map(r => r.name));
      const toAdd = allRoleObjs.filter(r => !existing.has(r.name));
      return [...list, ...toAdd];
    });
  }

  confirm(): void {
    this.dialogRef.close([...this.assignedListSig()]);
  }

  cancel(): void {
    this.dialogRef.close(undefined);
  }
}
