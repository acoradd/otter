import { EnvironmentView, Settings, WorkspaceItem, WorkspaceView } from '@bindings/apps/desktop';

import { FakeBackend } from './backend';
import appInfo from './fixtures/app-info.json';
import environmentView from './fixtures/environment-view.json';
import settings from './fixtures/settings.json';
import workspaceView from './fixtures/workspace-view.json';
import workspaces from './fixtures/workspaces.json';

/** Copies of the fixtures, safe to change in a test. */
export const fixtures = {
  settings: (): Settings => structuredClone(settings),
  workspaces: (): WorkspaceItem[] => structuredClone(workspaces),
  workspaceView: (): WorkspaceView => structuredClone(workspaceView),
  environmentView: (): EnvironmentView => structuredClone(environmentView),
};

/** Go with the demo workspace open ("Payments API", environment dev active). */
export function demoBackend(): FakeBackend {
  return new FakeBackend()
    .on('AppService.GetAppInfo', appInfo)
    .on('SettingsService.GetSettings', fixtures.settings())
    .on('WorkspaceService.ListWorkspaces', fixtures.workspaces())
    .on('WorkspaceService.GetWorkspaceView', fixtures.workspaceView())
    .on('WorkspaceService.GetEnvironmentView', fixtures.environmentView());
}

/** Go with no workspace yet. */
export function emptyBackend(): FakeBackend {
  return new FakeBackend()
    .on('AppService.GetAppInfo', appInfo)
    .on('SettingsService.GetSettings', { ...fixtures.settings(), lastWorkspaceId: 0 })
    .on('WorkspaceService.ListWorkspaces', []);
}
