import { CdkVirtualScrollViewport, ScrollingModule } from '@angular/cdk/scrolling';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { NodeKind, TreeNode } from '@bindings/apps/desktop';

import { NodeRef, ROOT, TreeRow, TreeStore, nodeKey, sameNode } from '../core/tree.store';
import { NameForm } from '../shared/name-form';
import { DropPosition, dropMove, dropPosition } from './drop';
import { MoveDialog } from './move-dialog';
import { TabsStore } from '../core/tabs.store';

/** Fixed row height of the virtual scroll, in px (tree.scss). */
export const ROW_HEIGHT = 26;
/** Pointer travel before a press becomes a drag, in px. */
const DRAG_THRESHOLD = 4;
/** Distance from the viewport edge that scrolls it during a drag, in px. */
const EDGE = 24;

interface Menu {
  x: number;
  y: number;
  /** null: the empty area of the tree (the root). */
  row: TreeRow | null;
}

interface Drag {
  node: NodeRef;
  name: string;
  x: number;
  y: number;
  active: boolean;
}

interface DropTarget {
  index: number;
  position: DropPosition;
  valid: boolean;
}

/**
 * The tree of the open workspace: a flat list of the visible rows in a
 * virtual scroll (fixed row height), so thousands of expanded nodes stay
 * smooth. Drag and drop uses pointer events.
 */
@Component({
  selector: 'app-tree',
  imports: [TranslocoDirective, ScrollingModule, NameForm, MoveDialog],
  templateUrl: './tree.html',
  styleUrl: './tree.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '(document:keydown.escape)': 'closeMenu()',
  },
})
export class Tree {
  protected readonly store = inject(TreeStore);
  private readonly tabs = inject(TabsStore);
  private readonly transloco = inject(TranslocoService);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly viewport = viewChild(CdkVirtualScrollViewport);

  protected readonly rowHeight = ROW_HEIGHT;
  protected readonly rows = this.store.rows;
  protected readonly menu = signal<Menu | null>(null);
  protected readonly confirmDelete = signal<TreeNode | null>(null);
  protected readonly moving = signal<TreeNode | null>(null);
  protected readonly drag = signal<Drag | null>(null);
  protected readonly drop = signal<DropTarget | null>(null);
  /** The row the menu was opened on, highlighted while it is open. */
  protected readonly menuKey = computed(() => {
    const row = this.menu()?.row;
    return row ? nodeKey(row.node) : '';
  });

  protected readonly trackRow = (_: number, row: TreeRow) => nodeKey(row.node);
  protected readonly isFolder = (node: TreeNode) => node.kind === NodeKind.NodeFolder;
  protected readonly key = nodeKey;
  protected readonly same = sameNode;

  /** The click that ends a drag must not also select. */
  private justDragged = false;

  constructor() {
    const move = (e: PointerEvent) => this.onPointerMove(e);
    const up = (e: PointerEvent) => void this.onPointerUp(e);
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    inject(DestroyRef).onDestroy(() => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
    });
  }

  // --- Selection, expansion, keyboard --------------------------------------

  protected onRowClick(row: TreeRow): void {
    if (this.justDragged) {
      this.justDragged = false;
      return;
    }
    this.store.selected.set(row.node);
    if (this.isFolder(row.node)) {
      this.store.toggle(row.node.id);
    } else {
      void this.tabs.open('request', row.node.id);
    }
  }

  protected onKeydown(event: KeyboardEvent): void {
    const selected = this.store.selected();
    const row = selected && this.rows().find((r) => sameNode(r.node, selected));
    if (!row || this.store.renaming()) {
      return;
    }
    if (event.key === 'F2') {
      event.preventDefault();
      this.store.renaming.set(row.node);
    } else if (event.key === 'Delete') {
      event.preventDefault();
      this.confirmDelete.set(row.node);
    }
  }

  // --- Context menu --------------------------------------------------------

  protected openMenu(event: MouseEvent, row: TreeRow | null): void {
    event.preventDefault();
    event.stopPropagation();
    if (row) {
      this.store.selected.set(row.node);
    }
    this.menu.set({ x: event.clientX, y: event.clientY, row });
  }

  closeMenu(): void {
    this.menu.set(null);
  }

  /** The folder new nodes go into from the menu: the row's folder, or the root. */
  private menuFolder(): number {
    const row = this.menu()?.row;
    if (!row) {
      return ROOT;
    }
    return this.isFolder(row.node) ? row.node.id : row.parentId;
  }

  protected newFolder(parentId = this.menuFolder()): void {
    this.closeMenu();
    void this.store.createFolder(parentId, this.transloco.translate('tree.newFolder'));
  }

  protected newRequest(): void {
    const folder = this.menuFolder();
    this.closeMenu();
    void this.store.createRequest(folder, this.transloco.translate('tree.newRequest'));
  }

  protected editFolder(node: TreeNode): void {
    this.closeMenu();
    void this.tabs.open('folder', node.id);
  }

  protected renameFromMenu(node: TreeNode): void {
    this.closeMenu();
    this.store.renaming.set(node);
  }

  protected duplicate(node: TreeNode): void {
    this.closeMenu();
    void this.store.duplicate(node, this.transloco.translate('tree.copyName', { name: node.name }));
  }

  protected askDelete(node: TreeNode): void {
    this.closeMenu();
    this.confirmDelete.set(node);
  }

  protected async doDelete(): Promise<void> {
    const node = this.confirmDelete();
    this.confirmDelete.set(null);
    if (node) {
      await this.store.delete(node);
    }
  }

  protected askMove(node: TreeNode): void {
    this.closeMenu();
    this.moving.set(node);
  }

  protected async moveTo(folderId: number): Promise<void> {
    const node = this.moving();
    this.moving.set(null);
    if (node) {
      await this.store.move(node, folderId, -1);
    }
  }

  // --- Drag and drop (pointer events) --------------------------------------

  protected onPointerDown(event: PointerEvent, row: TreeRow): void {
    if (event.button !== 0 || (event.target as HTMLElement).closest('input, app-name-form')) {
      return;
    }
    this.drag.set({
      node: row.node,
      name: row.node.name,
      x: event.clientX,
      y: event.clientY,
      active: false,
    });
  }

  private onPointerMove(event: PointerEvent): void {
    const drag = this.drag();
    if (!drag) {
      return;
    }
    if (!drag.active) {
      if (Math.hypot(event.clientX - drag.x, event.clientY - drag.y) < DRAG_THRESHOLD) {
        return;
      }
      this.closeMenu();
    }
    this.drag.set({ ...drag, active: true, x: event.clientX, y: event.clientY });
    this.autoScroll(event.clientY);
    this.drop.set(this.targetAt(event.clientX, event.clientY, drag.node));
  }

  private async onPointerUp(event: PointerEvent): Promise<void> {
    const drag = this.drag();
    this.drag.set(null);
    if (!drag?.active) {
      return;
    }
    this.justDragged = true;
    setTimeout(() => (this.justDragged = false));
    const target = this.targetAt(event.clientX, event.clientY, drag.node) ?? this.drop();
    this.drop.set(null);
    if (!target?.valid) {
      return;
    }
    const move = dropMove(
      this.rows(),
      this.store.children(),
      drag.node,
      target.index,
      target.position,
    );
    if (move) {
      await this.store.move(drag.node, move.parentId, move.index);
    }
  }

  /** The row under the pointer, where on it, and whether a drop there makes sense. */
  private targetAt(x: number, y: number, dragged: NodeRef): DropTarget | null {
    const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-row]');
    if (!el || !this.host.nativeElement.contains(el)) {
      return null;
    }
    const index = this.rows().findIndex((r) => nodeKey(r.node) === el.dataset['row']);
    if (index < 0) {
      return null;
    }
    const box = el.getBoundingClientRect();
    const position = dropPosition(this.rows()[index].node, (y - box.top) / box.height);
    const valid = dropMove(this.rows(), this.store.children(), dragged, index, position) !== null;
    return { index, position, valid };
  }

  private autoScroll(y: number): void {
    const viewport = this.viewport();
    if (!viewport) {
      return;
    }
    const box = viewport.elementRef.nativeElement.getBoundingClientRect();
    if (box.height === 0) {
      return;
    }
    if (y < box.top + EDGE) {
      viewport.scrollToOffset(viewport.measureScrollOffset() - ROW_HEIGHT);
    } else if (y > box.bottom - EDGE) {
      viewport.scrollToOffset(viewport.measureScrollOffset() + ROW_HEIGHT);
    }
  }

  protected dropClass(index: number): string {
    const drop = this.drop();
    if (!drop || drop.index !== index || !this.drag()?.active) {
      return '';
    }
    return drop.valid ? `drop-${drop.position}` : 'drop-refused';
  }
}
