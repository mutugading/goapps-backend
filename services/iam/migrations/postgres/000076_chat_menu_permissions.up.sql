-- 000076: Seed chat menu entries and permissions.

-- Level-2: Chat parent menu
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES (
    '00000000-0000-0000-0002-000000000017',
    NULL,
    'CHAT',
    'Chat',
    '/chat',
    'MessageSquare',
    'iam',
    2,
    17,
    TRUE,
    TRUE,
    'system'
) ON CONFLICT (menu_code) DO NOTHING;

-- Level-3: /chat page leaf
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES (
    '00000000-0000-0000-0003-000000000041',
    '00000000-0000-0000-0002-000000000017',
    'CHAT_PAGE',
    'All Conversations',
    '/chat',
    'MessageSquare',
    'iam',
    3,
    1,
    TRUE,
    TRUE,
    'system'
) ON CONFLICT (menu_code) DO NOTHING;

-- Permissions
INSERT INTO mst_permission (permission_id, permission_code, name, description, created_by)
VALUES
    (gen_random_uuid(), 'iam.chat.message.view',   'Chat View',           'View chat conversations and messages.', 'system'),
    (gen_random_uuid(), 'iam.chat.message.create', 'Chat Create Message', 'Send messages in chat.',               'system'),
    (gen_random_uuid(), 'iam.chat.message.delete', 'Chat Delete Message', 'Delete own messages in chat.',         'system'),
    (gen_random_uuid(), 'iam.chatbot.assistant.use', 'Chatbot Use',       'Use the AI chatbot assistant.',        'system')
ON CONFLICT (permission_code) DO NOTHING;

-- Wire menu → permission (CHAT_PAGE visible to anyone with chat.message.view)
INSERT INTO menu_permissions (id, menu_id, permission_id, assigned_by)
SELECT
    gen_random_uuid(),
    '00000000-0000-0000-0003-000000000041',
    permission_id,
    'system'
FROM mst_permission
WHERE permission_code = 'iam.chat.message.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Assign all chat permissions to SUPER_ADMIN role
INSERT INTO role_permissions (role_id, permission_id)
SELECT
    r.role_id,
    p.permission_id
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN (
      'iam.chat.message.view',
      'iam.chat.message.create',
      'iam.chat.message.delete',
      'iam.chatbot.assistant.use'
  )
ON CONFLICT DO NOTHING;
