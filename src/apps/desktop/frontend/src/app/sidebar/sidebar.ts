import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  output,
  signal,
} from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';

import { WorkspaceStore } from '../core/workspace.store';
import { NameForm } from '../shared/name-form';
import { Tree } from '../tree/tree';

type Mode = 'idle' | 'create' | 'rename' | 'delete';

/** The sidebar: workspace selector, environment selector, then the tree. */
@Component({
  selector: 'app-sidebar',
  imports: [TranslocoDirective, NameForm, Tree],
  templateUrl: './sidebar.html',
  styleUrl: './sidebar.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Sidebar {
  protected readonly store = inject(WorkspaceStore);
  readonly manageEnvironments = output<void>();

  protected readonly mode = signal<Mode>('idle');
  protected readonly view = this.store.view;
  protected readonly currentName = computed(() => this.view()?.name ?? '');

  protected async openWorkspace(value: string): Promise<void> {
    await this.store.open(Number(value));
  }

  protected async create(name: string): Promise<void> {
    if (await this.store.create(name)) {
      this.mode.set('idle');
    }
  }

  protected async rename(name: string): Promise<void> {
    if (await this.store.rename(this.store.currentId(), name)) {
      this.mode.set('idle');
    }
  }

  protected async delete(): Promise<void> {
    if (await this.store.delete(this.store.currentId())) {
      this.mode.set('idle');
    }
  }

  protected async setEnvironment(value: string): Promise<void> {
    await this.store.setActiveEnvironment(Number(value));
  }
}
