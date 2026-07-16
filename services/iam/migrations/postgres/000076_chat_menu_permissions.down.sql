-- 000076 down: Remove chat menu entries and permissions.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'iam.chat.message.view',
        'iam.chat.message.create',
        'iam.chat.message.delete',
        'iam.chatbot.assistant.view'
    )
);

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0001-000000000008',
    '00000000-0000-0000-0002-000000000017'
);

DELETE FROM mst_permission
WHERE permission_code IN (
    'iam.chat.message.view',
    'iam.chat.message.create',
    'iam.chat.message.delete',
    'iam.chatbot.assistant.view'
);

DELETE FROM mst_menu
WHERE menu_id IN (
    '00000000-0000-0000-0001-000000000008',
    '00000000-0000-0000-0002-000000000017'
);
