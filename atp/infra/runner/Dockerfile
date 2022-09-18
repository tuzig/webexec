FROM mcr.microsoft.com/playwright:v1.22.2-focal 
WORKDIR /app
ENV NODE_PATH="/usr/lib/node_modules"
ADD ./atp/infra/runner/package.json .
ADD ./atp/infra/runner/yarn.lock .
RUN yarn install --frozen-lockfile
CMD cp /runner/*.spec.ts . && npx playwright test ${PWARGS}
