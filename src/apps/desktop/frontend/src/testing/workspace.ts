import {
  EnvironmentView,
  FolderChoice,
  FolderView,
  HistoryItem,
  ResponseView,
  RequestView,
  Settings,
  TreeNode,
  WorkspaceItem,
  WorkspaceView,
} from '@bindings/apps/desktop';
import { Preview } from '@bindings/internal/resolve';

import { FakeBackend } from './backend';
import appInfo from './fixtures/app-info.json';
import environmentView from './fixtures/environment-view.json';
import folderChoices from './fixtures/folder-choices.json';
import folderView from './fixtures/folder-view.json';
import history from './fixtures/history.json';
import responseView from './fixtures/response-view.json';
import requestView from './fixtures/request-view.json';
import requestPreview from './fixtures/request-preview.json';
import folderPreview from './fixtures/folder-preview.json';
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
  /** "Create customer" (request 2) in customers, filled in. */
  requestView: (): RequestView => structuredClone(requestView),
  /** customers (folder 1), with a variable and an Authorization header. */
  folderView: (): FolderView => structuredClone(folderView),
  /** The preview of "Create customer" with the dev environment. */
  requestPreview: (): Preview => structuredClone(requestPreview) as Preview,
  /** The preview of customers (folder 1), as its editor holds it. */
  folderPreview: (): Preview => structuredClone(folderPreview) as Preview,
  /** "Create customer" sent: 201, a JSON body (fixed clock: 42 ms). */
  responseView: (): ResponseView => structuredClone(responseView),
  history: (): HistoryItem[] => structuredClone(history),
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
    .on('TreeService.GetFolderChoices', fixtures.folderChoices())
    .on('EditorService.GetRequestView', fixtures.requestView())
    .on('EditorService.GetFolderView', fixtures.folderView())
    .on('PreviewService.GetRequestPreview', fixtures.requestPreview())
    .on('PreviewService.GetFolderPreview', fixtures.folderPreview())
    .on('SendService.GetHistory', []);
}

/** Go with no workspace yet. */
export function emptyBackend(): FakeBackend {
  return new FakeBackend()
    .on('AppService.GetAppInfo', appInfo)
    .on('SettingsService.GetSettings', { ...fixtures.settings(), lastWorkspaceId: 0 })
    .on('WorkspaceService.ListWorkspaces', []);
}
