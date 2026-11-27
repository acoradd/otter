import { NodeKind, TreeNode } from '@bindings/apps/desktop';

import { TreeRow } from '../core/tree.store';
import { DropPosition, dropMove, dropPosition } from './drop';

const F = NodeKind.NodeFolder;
const R = NodeKind.NodeRequest;
const folder = (id: number, name: string): TreeNode => ({
  kind: F,
  id,
  name,
  method: '',
  hasChildren: true,
});
const request = (id: number, name: string): TreeNode => ({
  kind: R,
  id,
  name,
  method: 'GET',
  hasChildren: false,
});

// root: users (expanded: admin (expanded: purge), list, create), orders
const users = folder(1, 'users');
const orders = folder(2, 'orders');
const admin = folder(3, 'admin');
const list = request(10, 'list');
const create = request(11, 'create');
const purge = request(12, 'purge');
const children = new Map<number, TreeNode[]>([
  [0, [users, orders]],
  [1, [admin, list, create]],
  [3, [purge]],
]);
const row = (node: TreeNode, depth: number, parentId: number): TreeRow => ({
  node,
  depth,
  parentId,
  expanded: node.kind === F && children.has(node.id),
  loading: false,
});
const rows: TreeRow[] = [
  row(users, 0, 0), // 0
  row(admin, 1, 1), // 1
  row(purge, 2, 3), // 2
  row(list, 1, 1), // 3
  row(create, 1, 1), // 4
  row(orders, 0, 0), // 5
];
const at = (node: TreeNode) => rows.findIndex((r) => r.node === node);

describe('dropPosition', () => {
  it.each<[TreeNode, number, DropPosition]>([
    [users, 0.1, 'before'],
    [users, 0.5, 'inside'],
    [users, 0.9, 'after'],
    [list, 0.3, 'before'],
    [list, 0.7, 'after'],
    [users, NaN, 'inside'],
    [list, Infinity, 'after'],
  ])('%o at %d is %s', (node, ratio, want) => {
    expect(dropPosition(node, ratio)).toBe(want);
  });
});

describe('dropMove', () => {
  it.each<[string, TreeNode, TreeNode, DropPosition, ReturnType<typeof dropMove>]>([
    ['request before a sibling', create, list, 'before', { parentId: 1, index: 0 }],
    ['request after a sibling', list, create, 'after', { parentId: 1, index: 1 }],
    ['request on a folder goes inside, last', list, orders, 'before', { parentId: 2, index: -1 }],
    [
      'request next to a request of another folder',
      purge,
      list,
      'after',
      { parentId: 1, index: 1 },
    ],
    ['folder inside a folder', orders, admin, 'inside', { parentId: 3, index: -1 }],
    ['folder before a root folder', orders, users, 'before', { parentId: 0, index: 0 }],
    ['folder after a root folder', users, orders, 'after', { parentId: 0, index: 1 }],
    ['folder on a request goes into its folder', orders, list, 'after', { parentId: 1, index: -1 }],
    ['nested folder to the root', admin, orders, 'after', { parentId: 0, index: 2 }],
    ['on itself', list, list, 'after', null],
    ['folder into itself', users, users, 'inside', null],
    ['folder into its subtree', users, admin, 'inside', null],
    ['folder next to a request of its subtree', users, purge, 'before', null],
  ])('%s', (_, dragged, target, position, want) => {
    expect(
      dropMove(rows, children, { kind: dragged.kind, id: dragged.id }, at(target), position),
    ).toEqual(want);
  });

  it('refuses a missing target', () => {
    expect(dropMove(rows, children, { kind: R, id: 10 }, 99, 'after')).toBeNull();
  });
});
