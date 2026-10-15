import {useEffect, useState} from 'react';
import {
    AppShell,
    Avatar,
    Badge,
    Button,
    Group,
    Stack,
    Text,
    Card,
    SegmentedControl,
    Alert,
    Tabs,
    rem,
} from '@mantine/core';
import {
    IconServer2,
    IconActivity,
    IconSettings,
    IconPlayerPlay,
    IconPlayerStop,
    IconUserPlus,
} from '@tabler/icons-react';
import {AddAccount, LaunchAccount, ListAccounts, StartInstance, StopInstance} from '../wailsjs/go/main/App';

interface Account {
    name: string;
    username: string;
    running: boolean;
    display: string;
}

// Realistic placeholders for previewing in a plain browser (no Wails runtime) -
// exercises all three visual states: running+logged-in, stopped+logged-in, never
// logged in yet. Only used when window.go is absent, never in the real app.
const MOCK_ACCOUNTS: Account[] = [
    {name: 'test2', username: 'EasyOCR', running: true, display: ':11'},
    {name: 'test1', username: '3asyOCR', running: false, display: ''},
    {name: 'acct3', username: '', running: false, display: ''},
];

const inWails = () => typeof (window as any).go !== 'undefined';

function App() {
    const [accounts, setAccounts] = useState<Account[]>([]);
    const [backend, setBackend] = useState<'xvfb' | 'xephyr'>('xephyr');
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState<string | null>(null);

    const refresh = () => {
        if (!inWails()) {
            setAccounts(MOCK_ACCOUNTS);
            return;
        }
        ListAccounts()
            .then((result: any) => setAccounts(result ?? []))
            .catch((err: any) => setError(String(err)));
    };

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 5000);
        return () => clearInterval(interval);
    }, []);

    const runAction = (key: string, action: () => Promise<any>) => {
        if (typeof (window as any).go === 'undefined') {
            setError('Not running inside the Wails app - backend calls are unavailable in a plain browser.');
            return;
        }
        setBusy(key);
        setError(null);
        action()
            .then(refresh)
            .catch((err: any) => setError(String(err)))
            .finally(() => setBusy(null));
    };

    const handleAddAccount = () => runAction('__add__', () => AddAccount());

    const runningCount = accounts.filter((a) => a.running).length;
    const iconStyle = {width: rem(18), height: rem(18)};

    return (
        <AppShell padding={0} header={{height: 50}} footer={{height: 48}}>
            <Tabs
                defaultValue="instances"
                variant="pills"
                color="gray.1"
                style={{display: 'contents'}}
                styles={{tab: {color: 'var(--mantine-color-black)'}}}
            >
                <AppShell.Header withBorder>
                    <Tabs.List h="100%" px="md" style={{flexWrap: 'nowrap'}}>
                        <Tabs.Tab value="instances" leftSection={<IconServer2 style={iconStyle}/>}>
                            Accounts
                        </Tabs.Tab>
                        <Tabs.Tab value="status" leftSection={<IconActivity style={iconStyle}/>}>
                            Status
                        </Tabs.Tab>
                        <Tabs.Tab value="settings" leftSection={<IconSettings style={iconStyle}/>}>
                            Settings
                        </Tabs.Tab>
                    </Tabs.List>
                </AppShell.Header>

                <AppShell.Main>
                    <Tabs.Panel value="instances" p="md">
                        <Stack gap="md" maw={720} mx="auto">
                            {error && (
                                <Alert color="red" title="Something went wrong" withCloseButton
                                       onClose={() => setError(null)}>
                                    {error}
                                </Alert>
                            )}

                            <Card withBorder padding="lg" radius="md">
                                <Group justify="space-between">
                                    <Stack gap={2}>
                                        <Text fw={500}>Add a Roblox account</Text>
                                        <Text size="xs" c="dimmed">
                                            Opens a fresh window to log into a new account - no name needed, it's
                                            identified by whichever account you sign into.
                                        </Text>
                                    </Stack>
                                    <Button
                                        leftSection={<IconUserPlus style={{width: rem(16), height: rem(16)}}/>}
                                        loading={busy === '__add__'}
                                        onClick={handleAddAccount}
                                    >
                                        Add account
                                    </Button>
                                </Group>
                            </Card>

                            <Text fw={500}>Accounts</Text>

                            {accounts.length === 0 && (
                                <Text c="dimmed" size="sm">No accounts yet - add one above.</Text>
                            )}

                            {accounts.map((acct) => {
                                const loggedIn = acct.username !== '';
                                const label = loggedIn ? `@${acct.username}` : 'Not logged in yet';
                                return (
                                    <Card key={acct.name} withBorder padding="lg" radius="md">
                                        <Group justify="space-between">
                                            <Group gap="sm">
                                                <Avatar color={loggedIn ? 'blue' : 'gray'} radius="xl">
                                                    {loggedIn ? acct.username.slice(0, 2).toUpperCase() : '?'}
                                                </Avatar>
                                                <Stack gap={2}>
                                                    <Text fw={500} c={loggedIn ? undefined : 'dimmed'}>{label}</Text>
                                                    {acct.running ? (
                                                        <Badge color="blue" variant="light">running on {acct.display}</Badge>
                                                    ) : (
                                                        <Badge color="gray" variant="light">stopped</Badge>
                                                    )}
                                                </Stack>
                                            </Group>
                                            <Group>
                                                {acct.running ? (
                                                    <Button
                                                        color="red"
                                                        variant="light"
                                                        leftSection={<IconPlayerStop style={{width: rem(16), height: rem(16)}}/>}
                                                        loading={busy === acct.name}
                                                        onClick={() => runAction(acct.name, () => StopInstance(acct.name))}
                                                    >
                                                        Stop
                                                    </Button>
                                                ) : (
                                                    <Button
                                                        leftSection={<IconPlayerPlay style={{width: rem(16), height: rem(16)}}/>}
                                                        loading={busy === acct.name}
                                                        onClick={() => runAction(
                                                            acct.name,
                                                            () => loggedIn
                                                                ? LaunchAccount(acct.username)
                                                                : StartInstance(acct.name, 'xephyr'),
                                                        )}
                                                    >
                                                        {loggedIn ? 'Launch' : 'Log in'}
                                                    </Button>
                                                )}
                                            </Group>
                                        </Group>
                                    </Card>
                                );
                            })}
                        </Stack>
                    </Tabs.Panel>

                    <Tabs.Panel value="status" p="md">
                        <Stack gap="md" maw={720} mx="auto">
                            <Card withBorder padding="lg" radius="md">
                                <Stack gap="xs">
                                    <Text fw={500}>Overview</Text>
                                    <Group grow>
                                        <Stack gap={0} align="center">
                                            <Text size="xl" fw={700}>{accounts.length}</Text>
                                            <Text size="xs" c="dimmed">Total accounts</Text>
                                        </Stack>
                                        <Stack gap={0} align="center">
                                            <Text size="xl" fw={700} c="blue">{runningCount}</Text>
                                            <Text size="xs" c="dimmed">Running</Text>
                                        </Stack>
                                        <Stack gap={0} align="center">
                                            <Text size="xl" fw={700}>{accounts.length - runningCount}</Text>
                                            <Text size="xs" c="dimmed">Stopped</Text>
                                        </Stack>
                                    </Group>
                                </Stack>
                            </Card>
                            <Text size="sm" c="dimmed">
                                Per-instance GPU/VRAM usage isn't tracked yet - see PROJECT.md's open items.
                            </Text>
                        </Stack>
                    </Tabs.Panel>

                    <Tabs.Panel value="settings" p="md">
                        <Stack gap="md" maw={720} mx="auto">
                            <Card withBorder padding="lg" radius="md">
                                <Stack gap="sm">
                                    <Text fw={500}>Default launch backend</Text>
                                    <SegmentedControl
                                        value={backend}
                                        onChange={(v) => setBackend(v as 'xvfb' | 'xephyr')}
                                        data={[
                                            {label: 'Headless (Xvfb)', value: 'xvfb'},
                                            {label: 'Visible (Xephyr)', value: 'xephyr'},
                                        ]}
                                    />
                                    <Text size="xs" c="dimmed">
                                        Applies to newly-started instances. Each instance gets its own isolated X11
                                        display and metacity, matching the confirmed-working Flashback stack.
                                    </Text>
                                </Stack>
                            </Card>
                        </Stack>
                    </Tabs.Panel>
                </AppShell.Main>
            </Tabs>

            <AppShell.Footer withBorder>
                <Group h="100%" px="md" justify="space-between">
                    <Text size="sm">
                        Status <Text span c="dimmed">{runningCount > 0 ? `${runningCount} running` : 'Idle'}</Text>
                    </Text>
                    <Text size="sm" c="dimmed">RevolutionOS</Text>
                </Group>
            </AppShell.Footer>
        </AppShell>
    );
}

export default App;
