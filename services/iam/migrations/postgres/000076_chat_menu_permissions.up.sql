-- 000076: Seed chat menu entries and permissions.

-- Level-1: Chat root menu (top-level sidebar section, like Dashboard/Finance)
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES (
    '00000000-0000-0000-0001-000000000008',
    NULL,
    'CHAT',
    'Chat',
    NULL,
    'MessageSquare',
    'iam',
    1,
    80,
    TRUE,
    TRUE,
    'system'
) ON CONFLICT (menu_code) DO NOTHING;

-- Level-2: /chat page (child of Chat root)
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES (
    '00000000-0000-0000-0002-000000000017',
    '00000000-0000-0000-0001-000000000008',
    'CHAT_PAGE',
    'All Conversations',
    '/chat',
    'MessageSquare',
    'iam',
    2,
    1,
    TRUE,
    TRUE,
    'system'
) ON CONFLICT (menu_code) DO NOTHING;

-- Permissions
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, created_by)
VALUES
    (gen_random_uuid(), 'iam.chat.message.view',   'Chat View',           'View chat conversations and messages.',  'iam', 'chat', 'view',   'system'),
    (gen_random_uuid(), 'iam.chat.message.create', 'Chat Create Message', 'Send messages in chat.',                 'iam', 'chat', 'create', 'system'),
    (gen_random_uuid(), 'iam.chat.message.delete', 'Chat Delete Message', 'Delete own messages in chat.',           'iam', 'chat', 'delete', 'system'),
    (gen_random_uuid(), 'iam.chatbot.assistant.view', 'Chatbot Access',   'Access the AI chatbot assistant.',       'iam', 'chatbot', 'view', 'system')
ON CONFLICT (permission_code) DO NOTHING;

-- Wire menu → permission (CHAT_PAGE visible to anyone with chat.message.view)
INSERT INTO menu_permissions (id, menu_id, permission_id, assigned_by)
SELECT
    gen_random_uuid(),
    '00000000-0000-0000-0002-000000000017',
    permission_id,
    'system'
FROM mst_permission
WHERE permission_code = 'iam.chat.message.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Assign all chat permissions to SUPER_ADMIN role
INSERT INTO role_permissions (id, role_id, permission_id, assigned_by)
SELECT
    gen_random_uuid(),
    r.role_id,
    p.permission_id,
    'system'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN (
      'iam.chat.message.view',
      'iam.chat.message.create',
      'iam.chat.message.delete',
      'iam.chatbot.assistant.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;
