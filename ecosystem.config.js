module.exports = {
  apps: [
    {
      name: "orbit.api",
      cwd: __dirname,
      script: "./bin/orbit.api",
      args: "--mode=http",
      interpreter: "none",
      instances: 1,
      autorestart: true,
      max_restarts: 10,
    },
    {
      name: "orbit.billing",
      cwd: __dirname,
      script: "./bin/orbit.api",
      args: "--mode=billing",
      interpreter: "none",
      instances: 1,
      autorestart: true,
      max_restarts: 10,
    },
  ],
};
