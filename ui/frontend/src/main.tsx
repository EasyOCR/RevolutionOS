import React from 'react'
import {createRoot} from 'react-dom/client'
import '@mantine/core/styles.css'
import {MantineProvider, createTheme} from '@mantine/core'
import './style.css'
import App from './App'

// primaryColor/defaultColorScheme match a real reference screenshot of
// RevolutionMacro's actual UI (shared 2026-09-25) - light theme, blue accent.
// Earlier this only had extracted CSS tokens to go on (which suggested a dark
// default); the real screenshot is ground truth and overrides that guess.
const theme = createTheme({
    primaryColor: 'blue',
})

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <MantineProvider theme={theme} defaultColorScheme="light">
            <App/>
        </MantineProvider>
    </React.StrictMode>
)
