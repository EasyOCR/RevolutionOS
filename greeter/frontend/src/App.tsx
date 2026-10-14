import {useEffect, useState} from 'react';
import {Alert, Button, Paper, PasswordInput, Stack, Text, TextInput, Title} from '@mantine/core';
import {Login} from '../wailsjs/go/main/App';

const IN_WAILS = typeof (window as any).go !== 'undefined';

// Shown in the plain-browser live preview (see .claude/launch.json's
// greeter-dev entry) where window.go doesn't exist - lets the form layout
// and error states actually be iterated on without a full greetd backend.
// "wrong" as a password simulates a rejected login; anything else "succeeds"
// (logs to the console instead of actually starting a session).
async function mockLogin(username: string, password: string): Promise<string> {
    await new Promise((r) => setTimeout(r, 400));
    if (password === 'wrong') return 'Incorrect username or password.';
    console.log(`[preview] would start Flashback session for ${username}`);
    return '';
}

function App() {
    const [now, setNow] = useState(new Date());
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [loggingIn, setLoggingIn] = useState(false);

    useEffect(() => {
        const interval = setInterval(() => setNow(new Date()), 1000);
        return () => clearInterval(interval);
    }, []);

    const submit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!username || loggingIn) return;
        setLoggingIn(true);
        setError('');
        try {
            const result = IN_WAILS ? await Login(username, password) : await mockLogin(username, password);
            if (result) {
                setError(result);
                setPassword('');
            }
            // On success, greetd tears this process down to launch the
            // session - nothing more to do here.
        } catch (err) {
            setError('Could not reach the login service.');
        } finally {
            setLoggingIn(false);
        }
    };

    const dateStr = now.toLocaleDateString([], {weekday: 'long', month: 'long', day: 'numeric'});
    const timeStr = now.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});

    return (
        <div
            style={{
                height: '100vh',
                width: '100vw',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 32,
                WebkitUserSelect: 'none',
            }}
        >
            <Stack gap={2} align="center">
                <Text size="xl" fw={600}>{timeStr}</Text>
                <Text size="sm" c="dimmed">{dateStr}</Text>
            </Stack>

            <Paper component="form" onSubmit={submit} withBorder shadow="sm" radius="md" p="xl" style={{width: 340}}>
                <Stack gap="md">
                    <Title order={3} ta="center">RevolutionOS</Title>

                    <TextInput
                        label="Username"
                        value={username}
                        onChange={(e) => setUsername(e.currentTarget.value)}
                        autoFocus
                        disabled={loggingIn}
                        data-autofocus
                    />
                    <PasswordInput
                        label="Password"
                        value={password}
                        onChange={(e) => setPassword(e.currentTarget.value)}
                        disabled={loggingIn}
                    />

                    {error && (
                        <Alert
                            p="xs"
                            styles={{
                                root: {
                                    backgroundColor: 'rgba(250, 82, 82, 0.15)',
                                    border: '1px solid rgba(250, 82, 82, 0.4)',
                                },
                                message: {color: '#c92a2a'},
                            }}
                        >
                            {error}
                        </Alert>
                    )}

                    <Button type="submit" fullWidth loading={loggingIn} disabled={!username}>
                        Log In
                    </Button>
                </Stack>
            </Paper>

            {!IN_WAILS && <Text size="xs" c="dimmed">preview (mock login - password "wrong" simulates failure)</Text>}
        </div>
    );
}

export default App;
