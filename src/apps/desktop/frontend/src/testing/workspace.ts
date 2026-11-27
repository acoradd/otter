import {
  EnvironmentView,
  FolderChoice,
  Settings,
  TreeNode,
  WorkspaceItem,
  WorkspaceView,
} from '@bindings/apps/desktop';

import { FakeBackend } from './backend';
import appInfo from './fixtures/app-info.json';
import environmentView from './fixtures/environment-view.json';
import folderChoices from './fixtures/folder-choices.json';
import settings from './fixtures/settings.json';
import treeCards from './fixtures/tree-cards.json';
import treeCustomers from './fixtures/tree-customers.json';
import treeRoot from './fixtures/tree-root.json';
import workspaceView from './fixtures/workspace-view.json';
import workspaces from './fixtures/workspaces.json';

/** Copies of the fixtures, safe to change in a test. */
export const fixtures = {
  settings: (): Settings => structuredClone(settings),
  workspaces: (): WorkspaceItem[] => structuredClone(workspaces),
  workspaceView: (): WorkspaceView => structuredClone(workspaceView),
  environmentView: (): EnvironmentView => structuredClone(environmentView),
  /** Children by folder ID: root (0) holds customers (1), payments (2), webhooks (3); customers holds cards (4). */
  tree: (): Map<number, TreeNode[]> =>
    new Map([
      [0, structuredClone(treeRoot) as TreeNode[]],
      [1, structuredClone(treeCustomers) as TreeNode[]],
      [4, structuredClone(treeCards) as TreeNode[]],
    ]),
  folderChoices: (): FolderChoice[] => structuredClone(folderChoices),
};

/** Go with the demo workspace open ("Payments API", environment dev active). */
export function demoBackend(): FakeBackend {
  const tree = fixtures.tree();
  return new FakeBackend()
    .on('AppService.GetAppInfo', appInfo)
    .on('SettingsService.GetSettings', fixtures.settings())
    .on('WorkspaceService.ListWorkspaces', fixtures.workspaces())
    .on('WorkspaceService.GetWorkspaceView', fixtures.workspaceView())
    .on('WorkspaceService.GetEnvironmentView', fixtures.environmentView())
    .on('TreeService.GetChildren', (_ws: number, folder: number) => tree.get(folder) ?? [])
    .on('TreeService.GetFolderChoices', fixtures.folderChoices());
}

/** Go with no workspace yet. */
export function emptyBackend(): FakeBackend {
  return new FakeBackend()
    .on('AppService.GetAppInfo', appInfo)
    .on('SettingsService.GetSettings', { ...fixtures.settings(), lastWorkspaceId: 0 })
    .on('WorkspaceService.ListWorkspaces', []);
}
