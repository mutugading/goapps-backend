-- 000076 down: Remove chat menu entries and permissions.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'iam.chat.message.view',
        'iam.chat.message.create',
        'iam.chat.message.delete',
        'iam.chatbot.assistant.use'
    )
);

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0002-000000000017',
    '00000000-0000-0000-0003-000000000041'
);

DELETE FROM mst_permission
WHERE permission_code IN (
    'iam.chat.message.view',
    'iam.chat.message.create',
    'iam.chat.message.delete',
    'iam.chatbot.assistant.use'
);

DELETE FROM mst_menu
WHERE menu_id IN (
    '00000000-0000-0000-0002-000000000017',
    '00000000-0000-0000-0003-000000000041'
);
