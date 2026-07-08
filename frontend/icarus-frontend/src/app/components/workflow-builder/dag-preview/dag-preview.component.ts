import { Component, input, computed } from '@angular/core';

export interface FormNode {
  id: string;
  name: string;
  type: 'APPROVAL' | 'OPERATION';
  depends_on: string[];
  candidates: { type: 'USER' | 'ROLE'; value: string }[];
  policy: 'ANY' | 'ALL' | 'QUORUM';
  min_approvals: number;
  max_retries: number;
  retry_interval: number;
}

export interface DagLevel {
  level: number;
  nodes: FormNode[];
}

@Component({
  selector: 'app-dag-preview',
  templateUrl: './dag-preview.component.html',
  styleUrl: './dag-preview.component.scss',
})
export class DagPreviewComponent {
  readonly nodes = input<FormNode[]>([]);

  /** BFS topo-sort: assigns each node a level (depth from root). */
  readonly levels = computed<DagLevel[]>(() => {
    const nodeList = this.nodes();
    if (nodeList.length === 0) return [];

    const levelMap = new Map<string, number>();
    // Nodes with no dependencies are level 0.
    for (const n of nodeList) {
      if (n.depends_on.length === 0) {
        levelMap.set(n.id, 0);
      }
    }

    // Iteratively assign levels until stable.
    let changed = true;
    while (changed) {
      changed = false;
      for (const n of nodeList) {
        if (n.depends_on.length === 0) continue;
        const maxDepLevel = Math.max(...n.depends_on.map(dep => levelMap.get(dep) ?? 0));
        const desired = maxDepLevel + 1;
        if ((levelMap.get(n.id) ?? -1) < desired) {
          levelMap.set(n.id, desired);
          changed = true;
        }
      }
    }

    // Build level groups.
    const maxLevel = Math.max(...Array.from(levelMap.values()), 0);
    const result: DagLevel[] = [];
    for (let i = 0; i <= maxLevel; i++) {
      result.push({
        level: i,
        nodes: nodeList.filter(n => (levelMap.get(n.id) ?? 0) === i),
      });
    }
    return result;
  });

  /** Computes the critical path (longest chain by hop count). Returns a Set of node IDs on the path. */
  readonly criticalPath = computed<Set<string>>(() => {
    const nodeList = this.nodes();
    const levelMap = new Map<string, number>();
    for (const lvl of this.levels()) {
      for (const n of lvl.nodes) levelMap.set(n.id, lvl.level);
    }

    // Walk backwards from the node(s) at the deepest level.
    const maxLevel = Math.max(...Array.from(levelMap.values()), 0);
    const criticalSet = new Set<string>();
    const queue: string[] = nodeList
      .filter(n => (levelMap.get(n.id) ?? 0) === maxLevel)
      .map(n => n.id);

    const visited = new Set<string>();
    while (queue.length) {
      const id = queue.shift()!;
      if (visited.has(id)) continue;
      visited.add(id);
      criticalSet.add(id);
      const node = nodeList.find(n => n.id === id);
      if (node) {
        for (const dep of node.depends_on) {
          queue.push(dep);
        }
      }
    }
    return criticalSet;
  });

  /** Quick summary of how many parallel tracks exist (max nodes at any single level). */
  readonly parallelism = computed<number>(() => {
    return Math.max(...this.levels().map(l => l.nodes.length), 1);
  });

  isStageTransitionCritical(levelIndex: number): boolean {
    const currentLevels = this.levels();
    if (levelIndex <= 0 || levelIndex >= currentLevels.length) return false;

    const prevLevelNodes = currentLevels[levelIndex - 1].nodes;
    const currLevelNodes = currentLevels[levelIndex].nodes;
    const critical = this.criticalPath();

    // Is there any edge from a critical node in prevLevel to a critical node in currLevel?
    for (const currNode of currLevelNodes) {
      if (critical.has(currNode.id)) {
        for (const depId of currNode.depends_on) {
          if (critical.has(depId) && prevLevelNodes.some(n => n.id === depId)) {
            return true;
          }
        }
      }
    }
    return false;
  }

  candidateSummary(node: FormNode): string {
    if (node.type === 'OPERATION') return 'Automated';
    if (node.candidates.length === 0) return 'No candidates';
    const first = node.candidates.slice(0, 2).map(c => c.value || '—');
    const overflow = node.candidates.length - 2;
    const base = first.join(', ');
    return overflow > 0 ? `${base} +${overflow} more` : base;
  }

  policyLabel(node: FormNode): string {
    if (node.type === 'OPERATION') return '';
    if (node.policy === 'QUORUM') return `QUORUM (≥${node.min_approvals})`;
    return node.policy;
  }
}
