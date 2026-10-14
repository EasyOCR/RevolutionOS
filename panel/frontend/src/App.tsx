import {useEffect, useState} from 'react';
import {Button, Group, Menu, Text, rem} from '@mantine/core';
import {
    IconApps,
    IconFolder,
    IconLock,
    IconLogout,
    IconPower,
    IconRefresh,
} from '@tabler/icons-react';
import {
    ActivateTrayItem,
    ActivateWindow,
    LaunchApplication,
    ListApplications,
    ListPlaces,
    ListTrayItems,
    ListWindows,
    LockScreen,
    LogOut,
    OpenPlace,
    Restart,
    ShutDown,
} from '../wailsjs/go/main/App';
import type {main} from '../wailsjs/go/models';

const IN_WAILS = typeof (window as any).go !== 'undefined';

// Shown in the plain-browser live preview (see .claude/launch.json's
// revolutionos-panel-dev entry) where window.go doesn't exist - lets the
// dropdown/taskbar layout actually be iterated on without a full Wails build.
// Never shown when actually running inside Wails.
const MOCK_APPS: main.AppEntry[] = [
    {name: 'Files', exec: 'nautilus', categories: 'System;FileManager;'},
    {name: 'Terminal', exec: 'gnome-terminal', categories: 'System;TerminalEmulator;'},
    {name: 'Sober', exec: 'flatpak run org.vinegarhq.Sober', categories: 'Game;'},
];
const MOCK_PLACES: main.PlaceEntry[] = [
    {name: 'Home', path: '/home/revuser'},
    {name: 'Desktop', path: '/home/revuser/Desktop'},
    {name: 'Downloads', path: '/home/revuser/Downloads'},
];
const MOCK_WINDOWS: main.WindowEntry[] = [
    {id: 1, title: 'Sober'},
    {id: 2, title: 'RevolutionMacro'},
];
const MOCK_TRAY_ITEMS: main.TrayItem[] = [
    {service: ':1.42', path: '/StatusNotifierItem', title: 'Discord'},
];

const iconProps = {style: {width: rem(14), height: rem(14)}};

// Restart/ShutDown actually end the session they run in - only ever meant to
// be exercised inside a disposable VM, never the real host (see
// panel/system.go). The confirm() here is a deliberate extra speed bump
// against a stray click doing that for real, on top of that testing
// discipline - not a substitute for it.
const SYSTEM_ITEMS: {name: string; icon: React.ReactNode; danger?: boolean; action: () => void}[] = [
    {name: 'Lock Screen', icon: <IconLock {...iconProps}/>, action: () => IN_WAILS && LockScreen().catch(() => {})},
    {name: 'Log Out', icon: <IconLogout {...iconProps}/>, action: () => IN_WAILS && LogOut().catch(() => {})},
    {
        name: 'Restart',
        icon: <IconRefresh {...iconProps}/>,
        danger: true,
        action: () => {
            if (IN_WAILS && confirm('Restart now?')) Restart().catch(() => {});
        },
    },
    {
        name: 'Shut Down',
        icon: <IconPower {...iconProps}/>,
        danger: true,
        action: () => {
            if (IN_WAILS && confirm('Shut down now?')) ShutDown().catch(() => {});
        },
    },
];

function App() {
    const [now, setNow] = useState(new Date());
    const [apps, setApps] = useState<main.AppEntry[]>(IN_WAILS ? [] : MOCK_APPS);
    const [places, setPlaces] = useState<main.PlaceEntry[]>(IN_WAILS ? [] : MOCK_PLACES);
    const [windows, setWindows] = useState<main.WindowEntry[]>(IN_WAILS ? [] : MOCK_WINDOWS);
    const [trayItems, setTrayItems] = useState<main.TrayItem[]>(IN_WAILS ? [] : MOCK_TRAY_ITEMS);

    useEffect(() => {
        const interval = setInterval(() => setNow(new Date()), 1000);
        return () => clearInterval(interval);
    }, []);

    useEffect(() => {
        if (!IN_WAILS) return;
        const refreshWindows = () => ListWindows().then(setWindows).catch(() => setWindows([]));
        refreshWindows();
        const interval = setInterval(refreshWindows, 2000);
        return () => clearInterval(interval);
    }, []);

    useEffect(() => {
        if (!IN_WAILS) return;
        const refreshTray = () => ListTrayItems().then(setTrayItems).catch(() => setTrayItems([]));
        refreshTray();
        const interval = setInterval(refreshTray, 2000);
        return () => clearInterval(interval);
    }, []);

    const loadApps = () => IN_WAILS && ListApplications().then(setApps).catch(() => setApps([]));
    const loadPlaces = () => IN_WAILS && ListPlaces().then(setPlaces).catch(() => setPlaces([]));

    const launch = (app: main.AppEntry) => IN_WAILS && LaunchApplication(app.exec).catch(() => {});
    const openPlace = (place: main.PlaceEntry) => IN_WAILS && OpenPlace(place.path).catch(() => {});
    const activate = (win: main.WindowEntry) => IN_WAILS && ActivateWindow(win.id).catch(() => {});
    const activateTray = (item: main.TrayItem) => IN_WAILS && ActivateTrayItem(item.service, item.path).catch(() => {});

    const time = now.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});

    return (
        <Group
            h="100vh"
            w="100vw"
            gap={4}
            px={8}
            wrap="nowrap"
            style={{
                background: '#ffffff',
                borderBottom: '1px solid #dee2e6',
                boxSizing: 'border-box',
                WebkitUserSelect: 'none',
            }}
        >
            <Menu shadow="md" width={220} position="bottom-start" onOpen={loadApps}>
                <Menu.Target>
                    <Button variant="subtle" color="gray" size="compact-sm" leftSection={<IconApps {...iconProps}/>}>
                        Applications
                    </Button>
                </Menu.Target>
                <Menu.Dropdown>
                    {apps.length === 0 && <Menu.Item disabled>No applications found</Menu.Item>}
                    {apps.map((app) => (
                        <Menu.Item key={app.name} onClick={() => launch(app)}>
                            {app.name}
                        </Menu.Item>
                    ))}
                </Menu.Dropdown>
            </Menu>

            <Menu shadow="md" width={220} position="bottom-start" onOpen={loadPlaces}>
                <Menu.Target>
                    <Button variant="subtle" color="gray" size="compact-sm" leftSection={<IconFolder {...iconProps}/>}>
                        Places
                    </Button>
                </Menu.Target>
                <Menu.Dropdown>
                    {places.length === 0 && <Menu.Item disabled>No places found</Menu.Item>}
                    {places.map((place) => (
                        <Menu.Item key={place.name} onClick={() => openPlace(place)}>
                            {place.name}
                        </Menu.Item>
                    ))}
                </Menu.Dropdown>
            </Menu>

            <Group gap={4} wrap="nowrap" style={{flex: 1, overflow: 'hidden'}}>
                {windows.map((win) => (
                    <Button
                        key={win.id}
                        variant="light"
                        color="gray"
                        size="compact-sm"
                        onClick={() => activate(win)}
                        title={win.title}
                        styles={{label: {maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}}
                    >
                        {win.title}
                    </Button>
                ))}
            </Group>

            {trayItems.length > 0 && (
                <Group gap={4} wrap="nowrap">
                    {trayItems.map((item) => (
                        <Button
                            key={item.service + item.path}
                            variant="subtle"
                            color="gray"
                            size="compact-sm"
                            onClick={() => activateTray(item)}
                            title={item.title || item.service}
                        >
                            {item.title || item.service}
                        </Button>
                    ))}
                </Group>
            )}

            <Text size="sm">{time}</Text>

            <Menu shadow="md" width={200} position="bottom-end">
                <Menu.Target>
                    <Button variant="subtle" color="gray" size="compact-sm" leftSection={<IconPower {...iconProps}/>}>
                        System
                    </Button>
                </Menu.Target>
                <Menu.Dropdown>
                    {SYSTEM_ITEMS.map((item) => (
                        <Menu.Item key={item.name} leftSection={item.icon} color={item.danger ? 'red' : undefined} onClick={item.action}>
                            {item.name}
                        </Menu.Item>
                    ))}
                </Menu.Dropdown>
            </Menu>

            {!IN_WAILS && <Text size="xs" c="dimmed" ml={8}>preview (mock data)</Text>}
        </Group>
    );
}

export default App;
